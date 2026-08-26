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

	pointerDoSFixtureFilename        = "MaxMind-DB-test-pointer-decoder-dos.mmdb"
	pointerDoSIPv6FixtureFilename    = "MaxMind-DB-test-pointer-decoder-dos-ipv6.mmdb"
	pointerValueLimitFixtureFilename = "MaxMind-DB-test-decoder-value-limit-pointer-heavy.mmdb"
	payloadDoSFixtureFilename        = "MaxMind-DB-test-payload-amplification-dos.mmdb"
	worstCasePayloadFixtureFilename  = "MaxMind-DB-test-payload-amplification-dos-worst-case.mmdb"
	stringPayloadFixtureFilename     = "MaxMind-DB-test-payload-amplification-dos-string.mmdb"
	valueLimitFixtureFilename        = "MaxMind-DB-test-decoder-value-limit.mmdb"
	valueLimitOverFixtureFilename    = "MaxMind-DB-test-decoder-value-limit-over.mmdb"
	payloadLimitFixtureFilename      = "MaxMind-DB-test-decoder-payload-limit.mmdb"
	payloadLimitOverFixtureFilename  = "MaxMind-DB-test-decoder-payload-limit-over.mmdb"
	metadataLimitFixtureFilename     = "MaxMind-DB-test-metadata-payload-limit.mmdb"
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
	recommendedValueLimit   = 1 << 16
	recommendedPayloadLimit = 1 << 21

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

func writeScalar(buf []byte, size int, scalarType byte) int {
	pos := 1
	switch {
	case size <= 28:
		buf[0] = (scalarType << 5) | byte(size&0x1F)
	case size <= 284:
		buf[0] = (scalarType << 5) | 29
		buf[1] = byte(size - 29)
		pos++
	case size <= 65820:
		buf[0] = (scalarType << 5) | 30
		encoded := size - 285
		buf[1] = byte((encoded >> 8) & 0xFF)
		buf[2] = byte(encoded & 0xFF)
		pos += 2
	default:
		panic(fmt.Sprintf("scalar size %d is unsupported by this fixture writer", size))
	}
	clear(buf[pos : pos+size])
	return pos + size
}

func writeArrayHeader(buf []byte, size int) int {
	switch {
	case size <= 28:
		buf[0] = byte(size & 0x1F)
		buf[1] = 4
		return 2
	case size <= 284:
		buf[0] = 29
		buf[1] = 4
		buf[2] = byte(size - 29)
		return 3
	case size <= 65820:
		buf[0] = 30
		buf[1] = 4
		encoded := size - 285
		buf[2] = byte((encoded >> 8) & 0xFF)
		buf[3] = byte(encoded & 0xFF)
		return 4
	default:
		//nolint:gosec // size is a bounded fixture element count.
		return writeLargeArray(buf, uint32(size))
	}
}

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
	data = make([]byte, payloadScalarSize+16+pointerCount*2)
	pos := writeScalar(data, payloadScalarSize, scalarType)
	// The payload must not be left as NUL. A NUL-filled string measures length 0
	// through strlen, so a binding that copies C strings would skip the copy path
	// this fixture exists to exercise.
	for i := pos - payloadScalarSize; i < pos; i++ {
		data[i] = 'a'
	}

	top = pos
	pos += writeArrayHeader(data[pos:], pointerCount)
	for range pointerCount {
		pos += copy(data[pos:], writeDataPointer(0))
	}
	return data[:pos], top
}

// buildValueLimitData produces one array node followed by pointerCount scalar
// nodes. The scalar has no string/bytes payload, so only the decoded-value
// budget determines whether the record is accepted.
func buildValueLimitData(pointerCount int) (data []byte, top int) {
	data = make([]byte, 16+pointerCount*2)
	data[0] = 0xA0 // uint16 with value 0
	pos := 1
	top = pos
	pos += writeArrayHeader(data[pos:], pointerCount)
	for range pointerCount {
		pos += copy(data[pos:], writeDataPointer(0))
	}
	return data[:pos], top
}

// buildPayloadLimitData produces 32 references to a 65,535-byte value and one
// reference to a 32- or 33-byte value. Those totals are exactly 2 MiB and one
// byte over 2 MiB respectively, while the decoded-value count remains tiny.
func buildPayloadLimitData(smallSize int) (data []byte, top int) {
	const largePointerCount = 32
	data = make([]byte, payloadScalarSize+smallSize+128)
	pos := writeScalar(data, smallSize, scalarTypeBytes)
	largeOffset := pos
	pos += writeScalar(data[pos:], payloadScalarSize, scalarTypeBytes)
	top = pos
	pos += writeArrayHeader(data[pos:], largePointerCount+1)
	for range largePointerCount {
		pos += copy(data[pos:], writeDataPointer(largeOffset))
	}
	pos += copy(data[pos:], writeDataPointer(0))
	return data[:pos], top
}

// buildPayloadAmplificationDB builds a minimal valid IPv4 MMDB whose single
// search-tree node resolves every supported lookup to the payload
// amplification record. See buildPayloadAmplificationData.
func buildPayloadAmplificationDB(pointerCount int, scalarType byte) []byte {
	data, top := buildPayloadAmplificationData(pointerCount, scalarType)
	return buildSingleRecordDB(data, top)
}

func buildSingleRecordDB(data []byte, top int) []byte {
	const nodeCount = 1
	// A data record value is the data-section offset plus the node count plus the
	// 16-byte data section separator, which is how a reader recovers the offset:
	// offset = recordValue - nodeCount - dataSeparatorSize.
	recordValue := dataRecordValue24(nodeCount, top)

	buf := make([]byte, 1024+len(data))
	pos := 0
	pos += writeSearchTree(buf[pos:], recordValue)
	pos += dataSeparatorSize
	pos += copy(buf[pos:], data)
	pos += writeMetadataBlock(buf[pos:], nodeCount, pointerDoSBuildEpoch)
	return buf[:pos]
}

func buildValueLimitDB(pointerCount int) []byte {
	data, top := buildValueLimitData(pointerCount)
	return buildSingleRecordDB(data, top)
}

func buildPayloadLimitDB(smallSize int) []byte {
	data, top := buildPayloadLimitData(smallSize)
	return buildSingleRecordDB(data, top)
}

func writeMetadataLimitBlock(buf []byte, nodeCount uint32, buildEpoch uint64) int {
	pos := 0
	copy(buf[pos:], metadataMarker)
	pos += len(metadataMarker)
	metadataStart := pos
	pos += writeMap(buf[pos:], len(metadataKeysStandard))

	var databaseTypeOffset int
	for _, key := range metadataKeysStandard {
		pos += writeMetaKey(buf[pos:], key)
		switch key {
		case "binary_format_major_version":
			pos += writeUint16(buf[pos:], 2)
		case "binary_format_minor_version":
			pos += writeUint16(buf[pos:], 0)
		case "build_epoch":
			pos += writeUint64(buf[pos:], buildEpoch)
		case "database_type":
			databaseTypeOffset = pos - metadataStart
			pos += writeScalar(buf[pos:], payloadScalarSize, scalarTypeString)
			for i := pos - payloadScalarSize; i < pos; i++ {
				buf[i] = 'T'
			}
		case "description":
			pos += writeMap(buf[pos:], 0)
		case "ip_version":
			pos += writeUint16(buf[pos:], 4)
		case "languages":
			// The pointers below target the database_type string, so that key must
			// already be written. Offset 0 is the metadata map's own control byte,
			// which would silently make this a pointer cycle instead of a payload
			// fixture.
			if databaseTypeOffset == 0 {
				panic("languages must be written after database_type")
			}
			const pointerCount = 33
			pos += writeArrayHeader(buf[pos:], pointerCount)
			for range pointerCount {
				pos += copy(buf[pos:], writeDataPointer(databaseTypeOffset))
			}
		case "node_count":
			pos += writeUint32(buf[pos:], nodeCount)
		case "record_size":
			pos += writeUint16(buf[pos:], 24)
		default:
			panic("unknown metadata key: " + key)
		}
	}
	return pos
}

func buildMetadataLimitDB() []byte {
	const nodeCount = 1
	const recordValue = nodeCount + dataSeparatorSize
	buf := make([]byte, 128*1024)
	pos := writeSearchTree(buf, recordValue)
	pos += dataSeparatorSize
	pos += writeMap(buf[pos:], 0)
	pos += writeMetadataLimitBlock(buf[pos:], nodeCount, pointerDoSBuildEpoch)
	return buf[:pos]
}

// buildPointerFanOutDB builds a minimal valid IPv4 MMDB whose single search-tree
// node resolves every supported lookup to the fan-out data section. See
// buildPointerFanOutData.
func buildPointerFanOutDB(depth int) []byte {
	data, top := buildPointerFanOutData(depth)
	return buildSingleRecordDB(data, top)
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

// pointerValueLimitDepth is the deepest fan-out whose flat value count a reader
// at the recommended limit still accepts. A binary fan-out of depth d decodes
// 2**(d+1) - 1 values, so depth 15 gives 65,535, one below the limit, and depth
// 16 would give 131,071. Depth 15 is therefore the only depth that lands on the
// boundary from below.
const pointerValueLimitDepth = 15

// WritePointerDecoderDoSTestDB writes the databases that exercise the
// data-section pointer denial of service. Two exercise the fan-out: a minimal
// single-node database and a conventional IPv6 database that maps all of the
// address space to the fan-out record. Three exercise payload amplification: a
// moderate and a worst-case fixture with a shared bytes value, and one with a
// shared string value, the type most bindings copy into a native string. It
// also writes exact boundary fixtures for both recommended limits and a
// metadata fixture that exceeds the payload limit while opening. See
// buildPointerFanOutData and buildPayloadAmplificationData.
func (w *Writer) WritePointerDecoderDoSTestDB() error {
	files := map[string][]byte{
		pointerDoSFixtureFilename: buildPointerFanOutDB(pointerDoSDepth),
		pointerDoSIPv6FixtureFilename: buildPointerFanOutAllSpaceDB(
			pointerDoSDepth,
		),
		pointerValueLimitFixtureFilename: buildPointerFanOutDB(
			pointerValueLimitDepth,
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
		valueLimitFixtureFilename: buildValueLimitDB(
			recommendedValueLimit - 1,
		),
		valueLimitOverFixtureFilename: buildValueLimitDB(
			recommendedValueLimit,
		),
		payloadLimitFixtureFilename: buildPayloadLimitDB(
			recommendedPayloadLimit - 32*payloadScalarSize,
		),
		payloadLimitOverFixtureFilename: buildPayloadLimitDB(
			recommendedPayloadLimit - 32*payloadScalarSize + 1,
		),
		metadataLimitFixtureFilename: buildMetadataLimitDB(),
	}
	for name, db := range files {
		path := filepath.Clean(filepath.Join(w.target, name))
		if err := os.WriteFile(path, db, 0o644); err != nil {
			return fmt.Errorf("writing pointer DoS database %s: %w", name, err)
		}
	}
	return nil
}
