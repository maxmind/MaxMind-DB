package writer

import (
	"fmt"
	"os"
	"path/filepath"
)

// pointerDoSDepth is the nesting depth of the fan-out structure. Each level
// adds a factor of two to the number of leaf decodes a reader without pointer
// memoization performs. The 2**40 leaf decodes, and Θ(2**40) total decode
// operations, make a sub-kilobyte file impossible to decode without the fix
// while staying trivial to decode with it.
const pointerDoSDepth = 40

const (
	// pointerDoSBuildEpoch is fixed so the generated files are reproducible.
	pointerDoSBuildEpoch = 1_000_000_000

	pointerDoSFixtureFilename     = "MaxMind-DB-test-pointer-decoder-dos.mmdb"
	pointerDoSIPv6FixtureFilename = "MaxMind-DB-test-pointer-decoder-dos-ipv6.mmdb"
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
// data-section pointer fan-out denial of service: a minimal single-node
// database and a conventional IPv6 database that maps all of the address space
// to the fan-out record. See buildPointerFanOutData.
func (w *Writer) WritePointerDecoderDoSTestDB() error {
	files := map[string][]byte{
		pointerDoSFixtureFilename: buildPointerFanOutDB(pointerDoSDepth),
		pointerDoSIPv6FixtureFilename: buildPointerFanOutAllSpaceDB(
			pointerDoSDepth,
		),
	}
	for name, db := range files {
		path := filepath.Clean(filepath.Join(w.target, name))
		if err := os.WriteFile(path, db, 0o644); err != nil {
			return fmt.Errorf("writing pointer fan-out database %s: %w", name, err)
		}
	}
	return nil
}
