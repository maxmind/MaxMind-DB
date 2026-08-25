package writer

// Raw MMDB binary encoding helpers.
//
// This is a Go port of libmaxminddb's t/mmdb_test_writer.h, extended with
// large-size (case-31) encoding and complete database builders for crafting
// intentionally malformed MMDB files that cannot be created through mmdbwriter.

import "encoding/binary"

const (
	metadataMarker    = "\xab\xcd\xefMaxMind.com"
	dataSeparatorSize = 16
)

var (
	metadataKeysStandard = []string{
		"binary_format_major_version",
		"binary_format_minor_version",
		"build_epoch",
		"database_type",
		"description",
		"ip_version",
		"languages",
		"node_count",
		"record_size",
	}
	metadataKeysEmptyMapLast = []string{
		"binary_format_major_version",
		"binary_format_minor_version",
		"build_epoch",
		"database_type",
		"ip_version",
		"languages",
		"node_count",
		"record_size",
		"description",
	}
	metadataKeysEmptyArrayLast = []string{
		"binary_format_major_version",
		"binary_format_minor_version",
		"build_epoch",
		"database_type",
		"description",
		"ip_version",
		"node_count",
		"record_size",
		"languages",
	}
)

// writeMap writes a map control byte (type 7) for sizes <= 28.
func writeMap(buf []byte, size int) int {
	buf[0] = (7 << 5) | byte(size&0x1f)
	return 1
}

// writeString writes a string value (type 2).
func writeString(buf []byte, s string) int {
	buf[0] = (2 << 5) | byte(len(s)&0x1f)
	copy(buf[1:], s)
	return 1 + len(s)
}

// writeUint16 writes a uint16 value (type 5, 2 bytes).
func writeUint16(buf []byte, v uint16) int {
	buf[0] = (5 << 5) | 2
	binary.BigEndian.PutUint16(buf[1:], v)
	return 3
}

// writeUint32 writes a uint32 value (type 6, 4 bytes).
func writeUint32(buf []byte, v uint32) int {
	buf[0] = (6 << 5) | 4
	binary.BigEndian.PutUint32(buf[1:], v)
	return 5
}

// writeUint64 writes a uint64 value (extended type 9, 8 bytes).
func writeUint64(buf []byte, v uint64) int {
	buf[0] = (0 << 5) | 8
	buf[1] = 2 // extended type: 7 + 2 = 9 (uint64)
	binary.BigEndian.PutUint64(buf[2:], v)
	return 10
}

// writeMetaKey writes a metadata key as a string.
func writeMetaKey(buf []byte, key string) int {
	return writeString(buf, key)
}

// writeLargeArray writes an array control byte (extended type 11) with
// case-31 size encoding for sizes > 65820.
func writeLargeArray(buf []byte, size uint32) int {
	adjusted := size - 65821
	buf[0] = (0 << 5) | 31 // extended type, size = case 31
	buf[1] = 4             // extended type: 7 + 4 = 11 (array)
	buf[2] = byte((adjusted >> 16) & 0xFF)
	buf[3] = byte((adjusted >> 8) & 0xFF)
	buf[4] = byte(adjusted & 0xFF)
	return 5
}

// writeLargeMap writes a map control byte (type 7) with case-31 size
// encoding for sizes > 65820.
func writeLargeMap(buf []byte, size uint32) int {
	adjusted := size - 65821
	buf[0] = (7 << 5) | 31 // type 7 (map), size = case 31
	buf[1] = byte((adjusted >> 16) & 0xFF)
	buf[2] = byte((adjusted >> 8) & 0xFF)
	buf[3] = byte(adjusted & 0xFF)
	return 4
}

// writeEmptyArray writes a zero-length array (extended type 11).
func writeEmptyArray(buf []byte) int {
	buf[0] = 0 // extended type, size 0
	buf[1] = 4 // 7 + 4 = 11 (array)
	return 2
}

// writeSearchTree writes a 1-node search tree with 24-bit records,
// both pointing to the data section.
func writeSearchTree(buf []byte, recordValue uint32) int {
	return writeSearchTreeRecords(buf, recordValue, recordValue)
}

// writeSearchTreeRecords writes a 1-node search tree with 24-bit records
// where the left and right records can hold different values.
func writeSearchTreeRecords(buf []byte, leftRecord, rightRecord uint32) int {
	buf[0] = byte((leftRecord >> 16) & 0xFF)
	buf[1] = byte((leftRecord >> 8) & 0xFF)
	buf[2] = byte(leftRecord & 0xFF)
	buf[3] = byte((rightRecord >> 16) & 0xFF)
	buf[4] = byte((rightRecord >> 8) & 0xFF)
	buf[5] = byte(rightRecord & 0xFF)
	return 6
}

func writeMetadataBlockWithKeyOrder(
	buf []byte,
	nodeCount uint32,
	buildEpoch uint64,
	ipVersion uint16,
	keys []string,
) int {
	pos := 0

	copy(buf[pos:], metadataMarker)
	pos += len(metadataMarker)

	pos += writeMap(buf[pos:], len(keys))

	valueWriters := map[string]func([]byte) int{
		"binary_format_major_version": func(b []byte) int { return writeUint16(b, 2) },
		"binary_format_minor_version": func(b []byte) int { return writeUint16(b, 0) },
		"build_epoch":                 func(b []byte) int { return writeUint64(b, buildEpoch) },
		"database_type":               func(b []byte) int { return writeString(b, "Test") },
		"description":                 func(b []byte) int { return writeMap(b, 0) },
		"ip_version":                  func(b []byte) int { return writeUint16(b, ipVersion) },
		"languages":                   writeEmptyArray,
		"node_count":                  func(b []byte) int { return writeUint32(b, nodeCount) },
		"record_size":                 func(b []byte) int { return writeUint16(b, 24) },
	}

	for _, key := range keys {
		valueWriter, ok := valueWriters[key]
		if !ok {
			panic("unknown metadata key: " + key)
		}
		pos += writeMetaKey(buf[pos:], key)
		pos += valueWriter(buf[pos:])
	}

	return pos
}

// writeMetadataBlock writes the metadata marker followed by a standard
// metadata map with the given parameters.
func writeMetadataBlock(buf []byte, nodeCount uint32, buildEpoch uint64) int {
	return writeMetadataBlockWithKeyOrder(buf, nodeCount, buildEpoch, 4, metadataKeysStandard)
}

func buildSimpleDB(metadataWriter func([]byte, uint32, uint64) int) []byte {
	const nodeCount = 1
	const recordValue = nodeCount + 16
	const buildEpoch = 1_000_000_000

	buf := make([]byte, 1024)
	pos := 0

	pos += writeSearchTree(buf[pos:], recordValue)

	// 16-byte null separator
	pos += dataSeparatorSize

	// Data: a simple map with one string entry
	pos += writeMap(buf[pos:], 1)
	pos += writeString(buf[pos:], "ip")
	pos += writeString(buf[pos:], "test")

	pos += metadataWriter(buf[pos:], nodeCount, buildEpoch)

	return buf[:pos]
}

// buildOversizedArrayDB creates a complete MMDB with an array claiming
// 1,000,000 elements but containing only 2 actual entries.
func buildOversizedArrayDB() []byte {
	const nodeCount = 1
	const recordValue = nodeCount + 16

	buf := make([]byte, 1024)
	pos := 0

	pos += writeSearchTree(buf[pos:], recordValue)

	// 16-byte null separator
	pos += dataSeparatorSize

	// Data: array claiming 1M elements, only 2 strings present
	pos += writeLargeArray(buf[pos:], 1_000_000)
	pos += writeString(buf[pos:], "x")
	pos += writeString(buf[pos:], "y")

	pos += writeMetadataBlock(buf[pos:], nodeCount, 1_000_000_000)

	return buf[:pos]
}

// buildOversizedMapDB creates a complete MMDB with a map claiming
// 1,000,000 entries but containing only 1 key-value pair.
func buildOversizedMapDB() []byte {
	const nodeCount = 1
	const recordValue = nodeCount + 16

	buf := make([]byte, 1024)
	pos := 0

	pos += writeSearchTree(buf[pos:], recordValue)

	// 16-byte null separator
	pos += dataSeparatorSize

	// Data: map claiming 1M entries, only 1 k/v pair present
	pos += writeLargeMap(buf[pos:], 1_000_000)
	pos += writeString(buf[pos:], "k")
	pos += writeString(buf[pos:], "v")

	pos += writeMetadataBlock(buf[pos:], nodeCount, 1_000_000_000)

	return buf[:pos]
}

// buildUint64MaxEpochDB creates a complete MMDB with build_epoch set to
// UINT64_MAX (18446744073709551615). The database is structurally valid
// but the extreme epoch value can cause overflow in time conversions.
func buildUint64MaxEpochDB() []byte {
	return buildSimpleDB(func(buf []byte, nodeCount uint32, _ uint64) int {
		return writeMetadataBlock(buf, nodeCount, ^uint64(0))
	})
}

// writeMetadataBlockEmptyMapLast writes a metadata block where the last field
// is "description" (an empty map). This triggers the off-by-one bug where
// offset_to_next == data_section_size for a 0-length container.
func writeMetadataBlockEmptyMapLast(buf []byte, nodeCount uint32, buildEpoch uint64) int {
	return writeMetadataBlockWithKeyOrder(
		buf,
		nodeCount,
		buildEpoch,
		4,
		metadataKeysEmptyMapLast,
	)
}

// writeMetadataBlockEmptyArrayLast writes a metadata block where the last
// field is "languages" (an empty array).
func writeMetadataBlockEmptyArrayLast(buf []byte, nodeCount uint32, buildEpoch uint64) int {
	return writeMetadataBlockWithKeyOrder(
		buf,
		nodeCount,
		buildEpoch,
		4,
		metadataKeysEmptyArrayLast,
	)
}

// buildEmptyMapLastInMetadataDB creates a valid MMDB where the metadata
// map's last field is "description" (an empty map {}). This reproduces the
// off-by-one bug in get_entry_data_list() where offset == data_section_size
// is incorrectly rejected for 0-length containers.
func buildEmptyMapLastInMetadataDB() []byte {
	return buildSimpleDB(writeMetadataBlockEmptyMapLast)
}

// buildMetadataMarkerOnlyDB returns a file that contains only the metadata
// marker (\xab\xcd\xefMaxMind.com) with no metadata bytes following.
// Readers should reject this as invalid metadata rather than allowing a
// zero-length metadata section to reach the decoder.
func buildMetadataMarkerOnlyDB() []byte {
	return []byte(metadataMarker)
}

// buildSeparatorRecordDB creates a complete 1-node MMDB where the left and
// right records of node 0 hold the given values. With nodeCount = 1, any
// record value in the half-open range [2, 17) points into the 16-byte
// separator between the search tree and data section. Readers should reject
// such records as a corrupt search tree rather than exposing them as data
// entries with underflowed offsets.
func buildSeparatorRecordDB(leftRecord, rightRecord uint32) []byte {
	const nodeCount = 1
	const buildEpoch = 1_000_000_000

	buf := make([]byte, 1024)
	pos := 0

	pos += writeSearchTreeRecords(buf[pos:], leftRecord, rightRecord)

	// 16-byte null separator
	pos += dataSeparatorSize

	// Data section: a simple map so a valid record (nodeCount + 16) can
	// resolve to a real entry.
	pos += writeMap(buf[pos:], 1)
	pos += writeString(buf[pos:], "ip")
	pos += writeString(buf[pos:], "test")

	pos += writeMetadataBlock(buf[pos:], nodeCount, buildEpoch)

	return buf[:pos]
}

// buildSeparatorRecordMinLeftDB creates an MMDB whose node 0 left record
// equals nodeCount + 1 (the first byte of the data section separator).
func buildSeparatorRecordMinLeftDB() []byte {
	const nodeCount = 1
	const validRecord = nodeCount + dataSeparatorSize
	return buildSeparatorRecordDB(nodeCount+1, validRecord)
}

// buildSeparatorRecordMinRightDB creates an MMDB whose node 0 right record
// equals nodeCount + 1. The left record is valid, so this exercises the
// right-record-corruption path independently.
func buildSeparatorRecordMinRightDB() []byte {
	const nodeCount = 1
	const validRecord = nodeCount + dataSeparatorSize
	return buildSeparatorRecordDB(validRecord, nodeCount+1)
}

// buildSeparatorRecordMaxLeftDB creates an MMDB whose node 0 left record
// equals nodeCount + 15 (the last byte of the data section separator),
// exercising the upper boundary of the invalid range.
func buildSeparatorRecordMaxLeftDB() []byte {
	const nodeCount = 1
	const validRecord = nodeCount + dataSeparatorSize
	return buildSeparatorRecordDB(nodeCount+dataSeparatorSize-1, validRecord)
}

// buildEmptyArrayLastInMetadataDB creates a valid MMDB where the metadata
// map's last field is "languages" (an empty array []). Tests the array
// validation path of the same off-by-one bug.
func buildEmptyArrayLastInMetadataDB() []byte {
	return buildSimpleDB(writeMetadataBlockEmptyArrayLast)
}

// buildCorruptSearchTreeDB creates a complete MMDB where the metadata claims
// node_count = 100 but the actual search tree has only 1 node worth of real
// data (6 bytes for 24-bit records). The file is padded so MMDB_open
// succeeds (it validates file_size >= search_tree_size + separator), but
// MMDB_read_node with a node_number like 50 reads zeroed memory and should
// return MMDB_CORRUPT_SEARCH_TREE_ERROR.
func buildCorruptSearchTreeDB() []byte {
	const fakeNodeCount = 100
	const recordSize = 24
	// fakeNodeCount * (recordSize/4) = bytes in the fake search tree
	const fakeSearchTreeSize = fakeNodeCount * (recordSize / 4)
	const recordValue = fakeNodeCount + 16

	// Allocate enough for the fake tree + separator + data + metadata
	buf := make([]byte, fakeSearchTreeSize+dataSeparatorSize+1024)
	pos := 0

	// Write 1 real node at position 0; rest stays zeroed
	writeSearchTree(buf[pos:], recordValue)
	pos = fakeSearchTreeSize // skip to end of the fake tree

	// 16-byte null separator
	pos += dataSeparatorSize

	// Data: a simple map
	pos += writeMap(buf[pos:], 1)
	pos += writeString(buf[pos:], "ip")
	pos += writeString(buf[pos:], "test")

	// Metadata claims 100 nodes
	pos += writeMetadataBlock(buf[pos:], fakeNodeCount, 1_000_000_000)

	return buf[:pos]
}
