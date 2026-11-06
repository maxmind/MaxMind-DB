package writer

// Raw MMDB binary encoding helpers.
//
// This is a Go port of libmaxminddb's t/mmdb_test_writer.h, extended with
// large-size (case-31) encoding and complete database builders for crafting
// intentionally malformed MMDB files that cannot be created through mmdbwriter.

import (
	"encoding/binary"
	"fmt"
)

const (
	metadataMarker              = "\xab\xcd\xefMaxMind.com"
	dataSeparatorSize           = 16
	maximumSizeCode30           = 65820
	minimumSizeCode31           = maximumSizeCode30 + 1
	maximumDataStructureSize    = minimumSizeCode31 + (1 << 24) - 1
	maximum24BitSearchTreeValue = 1<<24 - 1
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

// writeMap writes a map control byte (type 7). Sizes above 28 use the extended
// size forms, which take one to three more bytes.
func writeMap(buf []byte, size int) int {
	if size < 0 || size > maximumDataStructureSize {
		panic(fmt.Sprintf(
			"map size %d is outside the supported range 0..%d",
			size,
			maximumDataStructureSize,
		))
	}
	switch {
	case size <= 28:
		buf[0] = (7 << 5) | byte(size&0x1F)
		return 1
	case size <= 284:
		buf[0] = (7 << 5) | 29
		buf[1] = byte(size - 29)
		return 2
	case size <= maximumSizeCode30:
		buf[0] = (7 << 5) | 30
		encoded := size - 285
		buf[1] = byte((encoded >> 8) & 0xFF)
		buf[2] = byte(encoded & 0xFF)
		return 3
	default:
		return writeLargeMap(buf, uint32(size))
	}
}

// writeString writes a string value (type 2). It delegates the control byte to
// writeScalar so strings longer than 28 bytes get the extended size forms
// instead of a truncated size.
func writeString(buf []byte, s string) int {
	pos := writeScalar(buf, len(s), scalarTypeString)
	copy(buf[pos-len(s):pos], s)
	return pos
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
	if size < minimumSizeCode31 || size > maximumDataStructureSize {
		panic(fmt.Sprintf(
			"array size %d is outside the case-31 range %d..%d",
			size,
			minimumSizeCode31,
			maximumDataStructureSize,
		))
	}
	adjusted := size - minimumSizeCode31
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
	if size < minimumSizeCode31 || size > maximumDataStructureSize {
		panic(fmt.Sprintf(
			"map size %d is outside the case-31 range %d..%d",
			size,
			minimumSizeCode31,
			maximumDataStructureSize,
		))
	}
	adjusted := size - minimumSizeCode31
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

// writeSearchTreeRecords writes one search-tree node with 24-bit records where
// the left and right records can hold different values.
func writeSearchTreeRecords(buf []byte, leftRecord, rightRecord uint32) int {
	if leftRecord > maximum24BitSearchTreeValue || rightRecord > maximum24BitSearchTreeValue {
		panic(fmt.Sprintf(
			"search-tree record values %d and %d must fit in 24 bits",
			leftRecord,
			rightRecord,
		))
	}
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
	return writeMetadataBlockWithEntries(
		buf,
		metadataEntries(nodeCount, buildEpoch, ipVersion, keys),
	)
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

// buildRepeatedMetadataKeyDB creates an MMDB whose metadata map has a second
// database_type key with the value "Other". A reader that keeps the first
// value sees "Test", and a reader that keeps the last value sees "Other".
// Readers should reject the database.
func buildRepeatedMetadataKeyDB() []byte {
	return buildSimpleDB(func(buf []byte, nodeCount uint32, buildEpoch uint64) int {
		entries := metadataEntries(nodeCount, buildEpoch, 4, metadataKeysStandard)
		entries = append(entries, metadataEntry{
			key:   "database_type",
			write: func(b []byte) int { return writeString(b, "Other") },
		})
		return writeMetadataBlockWithEntries(buf, entries)
	})
}

// buildRepeatedNodeCountDB creates an MMDB whose metadata map has a second
// node_count key with a different value. node_count sets the size of the
// search tree, so a reader that checks bounds with one value and reads the
// tree with the other can read the wrong bytes. Readers should reject the
// database.
func buildRepeatedNodeCountDB() []byte {
	return buildSimpleDB(func(buf []byte, nodeCount uint32, buildEpoch uint64) int {
		entries := metadataEntries(nodeCount, buildEpoch, 4, metadataKeysStandard)
		entries = append(entries, metadataEntry{
			key:   "node_count",
			write: func(b []byte) int { return writeUint32(b, nodeCount+1) },
		})
		return writeMetadataBlockWithEntries(buf, entries)
	})
}

// buildRepeatedPointerKeyDB creates an MMDB whose metadata map has a second
// database_type key with the value "Other". The second key is a pointer to the
// first key string, so a check that compares only the raw key bytes misses the
// repeat. Readers should reject the database.
func buildRepeatedPointerKeyDB() []byte {
	return buildSimpleDB(func(buf []byte, nodeCount uint32, buildEpoch uint64) int {
		entries := metadataEntries(nodeCount, buildEpoch, 4, metadataKeysStandard)
		entries = append(entries, metadataEntry{
			key:        "database_type",
			pointerKey: true,
			write:      func(b []byte) int { return writeString(b, "Other") },
		})
		return writeMetadataBlockWithEntries(buf, entries)
	})
}

// buildRepeatedDescriptionKeyDB creates an MMDB whose description map has
// the key "en" two times, with different values. Its languages array is
// ["en"]. Readers should reject the
// database.
func buildRepeatedDescriptionKeyDB() []byte {
	return buildSimpleDB(func(buf []byte, nodeCount uint32, buildEpoch uint64) int {
		entries := metadataEntries(nodeCount, buildEpoch, 4, metadataKeysStandard)
		replaceMetadataValue(entries, "description", writeRepeatedDescription)
		// List the description language, so the repeated key is the only
		// problem in the database.
		replaceMetadataValue(entries, "languages", func(b []byte) int {
			pos := writeArrayHeader(b, 1)
			return pos + writeString(b[pos:], "en")
		})
		return writeMetadataBlockWithEntries(buf, entries)
	})
}

// replaceMetadataValue sets the value writer of the entry with the given key.
// It panics if no entry has the key, so a builder cannot silently write a
// database without the case it is meant to contain.
func replaceMetadataValue(entries []metadataEntry, key string, write func([]byte) int) {
	for i := range entries {
		if entries[i].key == key {
			entries[i].write = write
			return
		}
	}
	panic("no metadata entry has the key " + key)
}

// writeRepeatedDescription writes {"en": "Test", "en": "Other"}.
func writeRepeatedDescription(buf []byte) int {
	pos := writeMap(buf, 2)
	pos += writeString(buf[pos:], "en")
	pos += writeString(buf[pos:], "Test")
	pos += writeString(buf[pos:], "en")
	pos += writeString(buf[pos:], "Other")
	return pos
}

// metadataEntry is one key of the metadata map and the function that writes
// its value. If pointerKey is true, the key is written as a pointer to the
// first key with the same name.
type metadataEntry struct {
	key        string
	pointerKey bool
	write      func([]byte) int
}

// metadataEntries returns the standard value writer for each key, in the
// order of keys.
func metadataEntries(
	nodeCount uint32,
	buildEpoch uint64,
	ipVersion uint16,
	keys []string,
) []metadataEntry {
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

	entries := make([]metadataEntry, 0, len(keys))
	for _, key := range keys {
		valueWriter, ok := valueWriters[key]
		if !ok {
			panic("unknown metadata key: " + key)
		}
		entries = append(entries, metadataEntry{key: key, write: valueWriter})
	}
	return entries
}

// writeMetadataBlockWithEntries writes the metadata marker followed by a map
// with the given entries. Entries can repeat a key.
func writeMetadataBlockWithEntries(buf []byte, entries []metadataEntry) int {
	pos := 0

	copy(buf[pos:], metadataMarker)
	pos += len(metadataMarker)

	metadataStart := pos
	pos += writeMap(buf[pos:], len(entries))

	keyOffsets := map[string]int{}
	for _, entry := range entries {
		if entry.pointerKey {
			offset, ok := keyOffsets[entry.key]
			if !ok {
				panic("no earlier metadata key to point to: " + entry.key)
			}
			pos += copy(buf[pos:], writeDataPointer(offset))
		} else {
			if _, ok := keyOffsets[entry.key]; !ok {
				keyOffsets[entry.key] = pos - metadataStart
			}
			pos += writeMetaKey(buf[pos:], entry.key)
		}
		pos += entry.write(buf[pos:])
	}

	return pos
}
