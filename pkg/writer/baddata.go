package writer

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"go4.org/netipx"
)

// WriteBadDataDBs writes intentionally corrupt or extreme MMDB databases
// for testing error handling in reader implementations.
func (w *Writer) WriteBadDataDBs(target string) error {
	//nolint:gosec // not security sensitive.
	if err := os.MkdirAll(target, os.ModePerm); err != nil {
		return fmt.Errorf("creating bad-data directory: %w", err)
	}

	// Raw binary databases — can't use mmdbwriter because the data is
	// intentionally invalid or uses values mmdbwriter can't represent.
	for _, db := range []struct {
		name string
		data []byte
	}{
		{"libmaxminddb-oversized-array.mmdb", buildOversizedArrayDB()},
		{"libmaxminddb-oversized-map.mmdb", buildOversizedMapDB()},
		{"libmaxminddb-uint64-max-epoch.mmdb", buildUint64MaxEpochDB()},
		{"libmaxminddb-corrupt-search-tree.mmdb", buildCorruptSearchTreeDB()},
	} {
		if err := writeRawDB(target, db.name, db.data); err != nil {
			return fmt.Errorf("writing %s: %w", db.name, err)
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
func writeDeepArrayNestingDB(dir string) error {
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
		if err := dbWriter.Insert(netipx.PrefixIPNet(prefix), value); err != nil {
			return fmt.Errorf("inserting %s: %w", cidr, err)
		}
	}

	path := filepath.Join(dir, "libmaxminddb-deep-array-nesting.mmdb")
	outputFile, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer outputFile.Close()

	if _, err := dbWriter.WriteTo(outputFile); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// writeDeepNestingDB creates an MMDB with 600 levels of nested maps.
// This exceeds libmaxminddb's MAXIMUM_DATA_STRUCTURE_DEPTH (512) and
// should trigger MMDB_INVALID_DATA_ERROR during data extraction.
func writeDeepNestingDB(dir string) error {
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
		if err := dbWriter.Insert(netipx.PrefixIPNet(prefix), value); err != nil {
			return fmt.Errorf("inserting %s: %w", cidr, err)
		}
	}

	path := filepath.Join(dir, "libmaxminddb-deep-nesting.mmdb")
	outputFile, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer outputFile.Close()

	if _, err := dbWriter.WriteTo(outputFile); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}
