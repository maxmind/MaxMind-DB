package writer

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/maxmind/mmdbwriter/v2"
	"github.com/maxmind/mmdbwriter/v2/mmdbtype"
)

// WriteBadDataDBs writes intentionally corrupt or extreme MMDB databases
// for testing error handling in reader implementations.
func (w *Writer) WriteBadDataDBs(target string) error {
	//nolint:gosec // not security sensitive.
	if err := os.MkdirAll(target, os.ModePerm); err != nil {
		return fmt.Errorf("creating bad-data directory: %w", err)
	}

	for name, data := range rawBadDataFixtures() {
		if err := writeRawDB(target, name, data); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	// Deep nesting uses mmdbwriter — structurally valid, just 600 levels deep.
	if err := writeDeepNestingDB(target); err != nil {
		return fmt.Errorf("writing deep nesting database: %w", err)
	}

	if err := writeDeepArrayNestingDB(target); err != nil {
		return fmt.Errorf("writing deep array nesting database: %w", err)
	}

	return nil
}

// rawBadDataFixtures returns the raw bad-data databases, keyed by filename.
// They are built byte by byte, because mmdbwriter cannot write invalid data or
// some extreme values. The writer and the byte-equality test share this map, so
// a new fixture cannot be added to one without the other.
func rawBadDataFixtures() map[string][]byte {
	return map[string][]byte{
		"libmaxminddb-oversized-array.mmdb":              buildOversizedArrayDB(),
		"libmaxminddb-oversized-map.mmdb":                buildOversizedMapDB(),
		"libmaxminddb-uint64-max-epoch.mmdb":             buildUint64MaxEpochDB(),
		"libmaxminddb-corrupt-search-tree.mmdb":          buildCorruptSearchTreeDB(),
		"libmaxminddb-empty-map-last-in-metadata.mmdb":   buildEmptyMapLastInMetadataDB(),
		"libmaxminddb-empty-array-last-in-metadata.mmdb": buildEmptyArrayLastInMetadataDB(),
		"libmaxminddb-metadata-marker-only.mmdb":         buildMetadataMarkerOnlyDB(),
		"libmaxminddb-separator-record-min-left.mmdb":    buildSeparatorRecordMinLeftDB(),
		"libmaxminddb-separator-record-min-right.mmdb":   buildSeparatorRecordMinRightDB(),
		"libmaxminddb-separator-record-max-left.mmdb":    buildSeparatorRecordMaxLeftDB(),
		"libmaxminddb-repeated-metadata-key.mmdb":        buildRepeatedMetadataKeyDB(),
		"libmaxminddb-repeated-description-key.mmdb":     buildRepeatedDescriptionKeyDB(),
		"libmaxminddb-repeated-node-count.mmdb":          buildRepeatedNodeCountDB(),
		"libmaxminddb-repeated-pointer-key.mmdb":         buildRepeatedPointerKeyDB(),
		"libmaxminddb-data-section-empty.mmdb": buildDataSectionDB(
			dataSectionEmptyRecord, dataSectionEmptyRecord, dataSeparatorSize, "",
		),
		"libmaxminddb-data-section-1-byte.mmdb": buildDataSectionDB(
			dataSectionFirstRecord, dataSectionEmptyRecord, dataSeparatorSize, "\xe0",
		),
		"libmaxminddb-data-section-2-bytes.mmdb": buildDataSectionDB(
			dataSectionFirstRecord, dataSectionEmptyRecord, dataSeparatorSize, "\x41a",
		),
		"libmaxminddb-data-section-truncated-size.mmdb": buildDataSectionDB(
			dataSectionFirstRecord, dataSectionEmptyRecord, dataSeparatorSize, truncatedSizeBoolean,
		),
		"libmaxminddb-data-ends-at-metadata-marker.mmdb": buildDataSectionDB(
			dataSectionFirstRecord, dataSectionEmptyRecord, dataSeparatorSize, "\x42ab",
		),
		// The string claims three bytes, but only two come before the marker.
		"libmaxminddb-data-extends-into-metadata-marker.mmdb": buildDataSectionDB(
			dataSectionFirstRecord, dataSectionEmptyRecord, dataSeparatorSize, "\x43ab",
		),
		// Only the last marker in the file starts the metadata.
		"libmaxminddb-data-contains-metadata-marker.mmdb": buildDataSectionDB(
			dataSectionFirstRecord,
			dataSectionEmptyRecord,
			dataSeparatorSize,
			"\x4e"+metadataMarker,
		),
		// The pointer targets data section offset 17, the first metadata byte.
		"libmaxminddb-pointer-into-metadata.mmdb": buildDataSectionDB(
			dataSectionFirstRecord, dataSectionEmptyRecord, dataSeparatorSize, "\x20\x11\x00",
		),
		// The right record targets data section offset 3, the marker.
		"libmaxminddb-record-points-at-metadata-marker.mmdb": buildDataSectionDB(
			dataSectionEmptyRecord, dataSectionFirstRecord+3, dataSeparatorSize, "\x42ab",
		),
		"libmaxminddb-no-data-separator.mmdb": buildDataSectionDB(
			dataSectionEmptyRecord, dataSectionEmptyRecord, 0, "",
		),
		"libmaxminddb-short-data-separator.mmdb": buildDataSectionDB(
			dataSectionEmptyRecord, dataSectionEmptyRecord, dataSeparatorSize/2, "",
		),
		// The size field of a boolean holds its value, so only 0 and 1 are valid.
		"libmaxminddb-boolean-size-2.mmdb": buildDataSectionDB(
			dataSectionFirstRecord, dataSectionEmptyRecord, dataSeparatorSize, "\x02\x07",
		),
		"libmaxminddb-search-tree-deeper-than-address.mmdb": buildDeepSearchTreeDB(),
		"libmaxminddb-no-search-tree.mmdb":                  buildNoSearchTreeDB(),
		"libmaxminddb-metadata-truncated-size.mmdb": []byte(
			metadataMarker + truncatedSizeBoolean,
		),
	}
}

func writeRawDB(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

// writeDeepArrayNestingDB creates an MMDB with 600 levels of nested arrays.
// This exceeds libmaxminddb's MAXIMUM_DATA_STRUCTURE_DEPTH (512) and
// should trigger MMDB_INVALID_DATA_ERROR during data extraction.
func writeDeepArrayNestingDB(dir string) (retErr error) {
	dbWriter, err := mmdbwriter.New(
		mmdbwriter.Options{
			DatabaseType: "Test",
			BuildEpoch:   1_000_000_000,
			IPVersion:    4,
			RecordSize:   24,
		},
	)
	if err != nil {
		return fmt.Errorf("creating mmdbwriter: %w", err)
	}

	// Build 600-level nested arrays: [[[... "x" ...]]]
	const depth = 600
	var value mmdbtype.DataType = mmdbtype.String("x")
	for range depth {
		value = mmdbtype.Slice{value}
	}

	for _, cidr := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("parsing prefix %s: %w", cidr, err)
		}
		if err := dbWriter.Insert(prefix, value); err != nil {
			return fmt.Errorf("inserting %s: %w", cidr, err)
		}
	}

	path := filepath.Join(dir, "libmaxminddb-deep-array-nesting.mmdb")
	outputFile, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer func() {
		if err := outputFile.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("closing file: %w", err)
		}
	}()

	if _, err := dbWriter.WriteTo(outputFile); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// writeDeepNestingDB creates an MMDB with 600 levels of nested maps.
// This exceeds libmaxminddb's MAXIMUM_DATA_STRUCTURE_DEPTH (512) and
// should trigger MMDB_INVALID_DATA_ERROR during data extraction.
func writeDeepNestingDB(dir string) (retErr error) {
	dbWriter, err := mmdbwriter.New(
		mmdbwriter.Options{
			DatabaseType: "Test",
			BuildEpoch:   1_000_000_000,
			IPVersion:    4,
			RecordSize:   24,
		},
	)
	if err != nil {
		return fmt.Errorf("creating mmdbwriter: %w", err)
	}

	// Build 600-level nested structure: {"a": {"a": ... "x" ...}}
	const depth = 600
	var value mmdbtype.DataType = mmdbtype.String("x")
	for range depth {
		value = mmdbtype.Map{"a": value}
	}

	// Insert for 0.0.0.0/1 and 128.0.0.0/1 to cover all IPv4 addresses.
	for _, cidr := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("parsing prefix %s: %w", cidr, err)
		}
		if err := dbWriter.Insert(prefix, value); err != nil {
			return fmt.Errorf("inserting %s: %w", cidr, err)
		}
	}

	path := filepath.Join(dir, "libmaxminddb-deep-nesting.mmdb")
	outputFile, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer func() {
		if err := outputFile.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("closing file: %w", err)
		}
	}()

	if _, err := dbWriter.WriteTo(outputFile); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}
