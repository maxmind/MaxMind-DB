package writer

import (
	"fmt"
	"os"
	"path/filepath"
)

// pointerDoSDepth is the nesting depth of the fan-out structure. Each level
// adds a factor of two to the number of leaf decodes an unprotected reader that
// re-decodes every target per referencing path performs. The 2**40 leaf
// decodes, and Θ(2**40) total decode operations, make a sub-kilobyte file
// impossible to decode that way. Memoization is one defense. A cumulative work
// budget or a schema-directed path also avoids the blow-up.
const pointerDoSDepth = 40

const (
	// pointerDoSBuildEpoch is fixed so the generated files are reproducible.
	pointerDoSBuildEpoch = 1_000_000_000

	pointerDoSFixtureFilename       = "MaxMind-DB-test-pointer-decoder-dos.mmdb"
	pointerDoSIPv6FixtureFilename   = "MaxMind-DB-test-pointer-decoder-dos-ipv6.mmdb"
	payloadDoSFixtureFilename       = "MaxMind-DB-test-payload-amplification-dos.mmdb"
	worstCasePayloadFixtureFilename = "MaxMind-DB-test-payload-amplification-dos-worst-case.mmdb"
	stringPayloadFixtureFilename    = "MaxMind-DB-test-payload-amplification-dos-string.mmdb"
)

// writeDataPointer writes a data-section pointer with a one-byte payload,
// which addresses offsets 0..2047. The value is relative to the start of the
// data section, which is where the reader sets the pointer base.
func writeDataPointer(target int) []byte {
	if target < 0 || target > 2047 {
		panic(fmt.Sprintf(
			"data pointer target %d is out of range for a one-byte payload (0..2047)",
			target,
		))
	}
	return []byte{
		(1 << 5) | byte((target>>8)&0x7),
		byte(target & 0xFF),
	}
}

// dataRecordValue24 converts a data-section offset to a 24-bit search-tree
// record value. A search-tree record value is biased by the node count and the
// data-section separator, so it is not the bare data-section offset that a
// data-section pointer uses. This adds that bias.
func dataRecordValue24(nodeCount uint32, dataOffset int) uint32 {
	if dataOffset < 0 {
		panic(fmt.Sprintf("negative data-section offset: %d", dataOffset))
	}
	recordValue := uint64(nodeCount) + dataSeparatorSize + uint64(dataOffset)
	const max24BitRecordValue = 1<<24 - 1
	if recordValue > max24BitRecordValue {
		panic(fmt.Sprintf(
			"data record value %d does not fit in a 24-bit search-tree record",
			recordValue,
		))
	}
	return uint32(recordValue)
}

// buildPointerFanOutData builds the data section: nested arrays, each holding
// two pointers to the node below. It is laid out leaf first, so the leaf is at
// offset 0 and each array points back to the level below it. A decoder that
// re-decodes a shared pointer target once per referencing path performs
// 2**depth leaf decodes and Θ(2**depth) total decode operations from these few
// hundred bytes (GHSA-hj94-g986-h9r7); a decoder that memoizes resolved targets
// decodes it in linear time. mmdbwriter cannot produce this shape because its
// own deduplication fans out while computing the file, so the bytes are written
// directly. top is the data-section offset of the outermost array.
func buildPointerFanOutData(depth int) (data []byte, top int) {
	data = []byte{0xA0} // uint16 with value 0
	prev := 0
	for range depth {
		offset := len(data)
		data = append(data, 0x02, 0x04) // array (extended type 11), size 2
		data = append(data, writeDataPointer(prev)...)
		data = append(data, writeDataPointer(prev)...)
		prev = offset
	}
	return data, prev
}

const (
	// payloadScalarSize is the size of the single shared bytes value that the
	// pointers target. Size code 30 covers 285..65820 bytes.
	payloadScalarSize = 1<<16 - 1
	// payloadPointerCount pointers each re-materialize the shared value, so a
	// reader that copies the target for every pointer allocates
	// payloadPointerCount*payloadScalarSize bytes, about 512 MiB, from a file of
	// a few tens of kilobytes.
	payloadPointerCount = 8192
	// worstCasePointerCount is the largest fan-out a reader whose only defense is
	// the value count still accepts. Under flat value accounting the array plus
	// its elements decode to worstCasePointerCount+1 = 65,536 values, which meets
	// the recommended limit without exceeding it, so that reader does not reject
	// it. Copying each target materializes worstCasePointerCount*payloadScalarSize
	// bytes, about 4 GiB, from a file of about 200 KiB. A limit on the copied
	// bytes stops this, as can safe reuse or structural rejection.
	worstCasePointerCount = 65535
)

// MMDB scalar type codes used by the payload fixtures: 2 is a UTF-8 string and
// 4 is bytes.
const (
	scalarTypeString byte = 2
	scalarTypeBytes  byte = 4
)

// buildPayloadAmplificationData builds a data section holding one large scalar
// value (string or bytes, per scalarType) at offset 0 followed by an array of
// pointers that all target it. The value count and depth limits do not bound
// this: the array stays well under the value limit, yet a reader that copies
// each pointer's target materializes payloadPointerCount*payloadScalarSize
// bytes. top is the offset of the array.
func buildPayloadAmplificationData(
	pointerCount int,
	scalarType byte,
) (data []byte, top int) {
	const sizeCode30 = 30
	encoded := payloadScalarSize - 285
	data = append(
		data,
		(scalarType<<5)|sizeCode30,
		byte((encoded>>8)&0xFF),
		byte(encoded&0xFF),
	)
	data = append(data, make([]byte, payloadScalarSize)...)

	top = len(data)
	arrEncoded := pointerCount - 285
	data = append(data, 0x1E, 0x04, byte((arrEncoded>>8)&0xFF), byte(arrEncoded&0xFF))
	for range pointerCount {
		data = append(data, writeDataPointer(0)...)
	}
	return data, top
}

// buildPayloadAmplificationDB builds a minimal valid IPv4 MMDB whose single
// search-tree node resolves every supported lookup to the payload
// amplification record. See buildPayloadAmplificationData.
func buildPayloadAmplificationDB(pointerCount int, scalarType byte) []byte {
	data, top := buildPayloadAmplificationData(pointerCount, scalarType)

	const nodeCount = 1
	recordValue := dataRecordValue24(nodeCount, top)

	buf := make([]byte, 1024+len(data))
	pos := 0
	pos += writeSearchTree(buf[pos:], recordValue)
	pos += dataSeparatorSize
	pos += copy(buf[pos:], data)
	pos += writeMetadataBlock(buf[pos:], nodeCount, pointerDoSBuildEpoch)
	return buf[:pos]
}

// buildPointerFanOutDB builds a minimal valid IPv4 MMDB whose single search-tree
// node resolves every supported lookup to the fan-out data section. See
// buildPointerFanOutData.
func buildPointerFanOutDB(depth int) []byte {
	data, top := buildPointerFanOutData(depth)

	const nodeCount = 1
	// A data record value is the data-section offset plus the node count plus
	// the 16-byte data section separator, which is how a reader recovers the
	// offset: offset = recordValue - nodeCount - dataSeparatorSize.
	recordValue := dataRecordValue24(nodeCount, top)

	buf := make([]byte, 1024+len(data))
	pos := 0
	pos += writeSearchTree(buf[pos:], recordValue)
	pos += dataSeparatorSize
	pos += copy(buf[pos:], data)
	pos += writeMetadataBlock(buf[pos:], nodeCount, pointerDoSBuildEpoch)
	return buf[:pos]
}

// buildPointerFanOutAllSpaceDB builds a conventional IPv6 MMDB that maps the
// entire address space to the fan-out data record, so opening it, looking up
// any address, and decoding the result reproduces the denial of service. A
// lookup only finds the record; decoding it is what expands the fan-out. This
// is the form a third-party implementation is most likely to test, since
// implementations commonly test with database files rather than bare data
// sections.
//
// The search tree is a spine down the all-zeros path. At every node the
// one-branch resolves to the data record and the zero-branch descends to the
// next node, and the final node resolves both branches to the record. The spine
// is 96 nodes deep so that an IPv4 lookup, which follows the ::0/96 prefix,
// reaches the final node and the record too.
func buildPointerFanOutAllSpaceDB(depth int) []byte {
	data, top := buildPointerFanOutData(depth)

	const ipv4PrefixBits = 96
	const nodeCount = ipv4PrefixBits + 1 // nodes 0..96
	badRecord := dataRecordValue24(nodeCount, top)

	const recordPairSize = 6 // two 24-bit records per node
	buf := make([]byte, nodeCount*recordPairSize+dataSeparatorSize+len(data)+256)
	pos := 0
	for i := range uint32(nodeCount) {
		leftRecord := badRecord
		if i < ipv4PrefixBits {
			leftRecord = i + 1 // descend the all-zeros path
		}
		pos += writeSearchTreeRecords(buf[pos:], leftRecord, badRecord)
	}
	pos += dataSeparatorSize
	pos += copy(buf[pos:], data)
	pos += writeMetadataBlockWithKeyOrder(
		buf[pos:],
		nodeCount,
		pointerDoSBuildEpoch,
		6,
		metadataKeysStandard,
	)
	return buf[:pos]
}

// WritePointerDecoderDoSTestDB writes the databases that exercise the
// data-section pointer denial of service. Two exercise the fan-out: a minimal
// single-node database and a conventional IPv6 database that maps all of the
// address space to the fan-out record. Three exercise payload amplification: a
// moderate and a worst-case fixture with a shared bytes value, and one with a
// shared string value, the type most bindings copy into a native string. See
// buildPointerFanOutData and buildPayloadAmplificationData.
func (w *Writer) WritePointerDecoderDoSTestDB() error {
	files := map[string][]byte{
		pointerDoSFixtureFilename: buildPointerFanOutDB(pointerDoSDepth),
		pointerDoSIPv6FixtureFilename: buildPointerFanOutAllSpaceDB(
			pointerDoSDepth,
		),
		payloadDoSFixtureFilename: buildPayloadAmplificationDB(
			payloadPointerCount,
			scalarTypeBytes,
		),
		worstCasePayloadFixtureFilename: buildPayloadAmplificationDB(
			worstCasePointerCount,
			scalarTypeBytes,
		),
		stringPayloadFixtureFilename: buildPayloadAmplificationDB(
			payloadPointerCount,
			scalarTypeString,
		),
	}
	for name, db := range files {
		path := filepath.Clean(filepath.Join(w.target, name))
		if err := os.WriteFile(path, db, 0o644); err != nil {
			return fmt.Errorf("writing pointer DoS database %s: %w", name, err)
		}
	}
	return nil
}
