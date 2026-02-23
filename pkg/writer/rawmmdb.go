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
	buf[0] = byte((recordValue >> 16) & 0xFF)
	buf[1] = byte((recordValue >> 8) & 0xFF)
	buf[2] = byte(recordValue & 0xFF)
	buf[3] = byte((recordValue >> 16) & 0xFF)
	buf[4] = byte((recordValue >> 8) & 0xFF)
	buf[5] = byte(recordValue & 0xFF)
	return 6
}

// writeMetadataBlock writes the metadata marker followed by a standard
// metadata map with the given parameters.
func writeMetadataBlock(buf []byte, nodeCount uint32, buildEpoch uint64) int {
	pos := 0

	copy(buf[pos:], metadataMarker)
	pos += len(metadataMarker)

	pos += writeMap(buf[pos:], 9)

	pos += writeMetaKey(buf[pos:], "binary_format_major_version")
	pos += writeUint16(buf[pos:], 2)

	pos += writeMetaKey(buf[pos:], "binary_format_minor_version")
	pos += writeUint16(buf[pos:], 0)

	pos += writeMetaKey(buf[pos:], "build_epoch")
	pos += writeUint64(buf[pos:], buildEpoch)

	pos += writeMetaKey(buf[pos:], "database_type")
	pos += writeString(buf[pos:], "Test")

	pos += writeMetaKey(buf[pos:], "description")
	pos += writeMap(buf[pos:], 0)

	pos += writeMetaKey(buf[pos:], "ip_version")
	pos += writeUint16(buf[pos:], 4)

	pos += writeMetaKey(buf[pos:], "languages")
	pos += writeEmptyArray(buf[pos:])

	pos += writeMetaKey(buf[pos:], "node_count")
	pos += writeUint32(buf[pos:], nodeCount)

	pos += writeMetaKey(buf[pos:], "record_size")
	pos += writeUint16(buf[pos:], 24)

	return pos
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
	const nodeCount = 1
	const recordValue = nodeCount + 16

	buf := make([]byte, 1024)
	pos := 0

	pos += writeSearchTree(buf[pos:], recordValue)

	// 16-byte null separator
	pos += dataSeparatorSize

	// Data: a simple map with one string entry
	pos += writeMap(buf[pos:], 1)
	pos += writeString(buf[pos:], "ip")
	pos += writeString(buf[pos:], "test")

	pos += writeMetadataBlock(buf[pos:], nodeCount, ^uint64(0))

	return buf[:pos]
}
