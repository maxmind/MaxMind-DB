// write-test-data generates test mmdb files.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxmind/MaxMind-DB/pkg/writer"
)

const moduleName = "github.com/maxmind/MaxMind-DB"

func main() {
	var defaultSource, defaultTarget, defaultBadData string
	if root, err := findRepoRoot(); err == nil {
		defaultSource = filepath.Join(root, "source-data")
		defaultTarget = filepath.Join(root, "test-data")
		defaultBadData = filepath.Join(root, "bad-data", "libmaxminddb")
	}

	source := flag.String("source", defaultSource, "Source data directory")
	target := flag.String(
		"target",
		defaultTarget,
		"Destination directory for the generated mmdb files",
	)
	badData := flag.String(
		"bad-data",
		defaultBadData,
		"Destination directory for generated bad mmdb files",
	)

	flag.Parse()

	w, err := writer.New(*source, *target)
	if err != nil {
		fmt.Printf("creating writer: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteIPv4TestDB(); err != nil {
		fmt.Printf("writing IPv4 test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteIPv6TestDB(); err != nil {
		fmt.Printf("writing IPv6 test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteMixedIPTestDB(); err != nil {
		fmt.Printf("writing IPv6 test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteNoIPv4TestDB(); err != nil {
		fmt.Printf("writing no IPv4 test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteNoMapTestDB(); err != nil {
		fmt.Printf("writing no map test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteMetadataPointersTestDB(); err != nil {
		fmt.Printf("writing metadata pointers test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteDecoderTestDB(); err != nil {
		fmt.Printf("writing decoder test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteDeeplyNestedStructuresTestDB(); err != nil {
		fmt.Printf("writing decoder test databases: %+v\n", err)
		os.Exit(1)
	}

	if err := w.WriteGeoIP2TestDB(); err != nil {
		fmt.Printf("writing GeoIP2 test databases: %+v\n", err)
		os.Exit(1)
	}

	if *badData != "" {
		if err := w.WriteBadDataDBs(*badData); err != nil {
			fmt.Printf("writing bad data test databases: %+v\n", err)
			os.Exit(1)
		}
	}
}

// findRepoRoot walks up from the current working directory looking for a
// go.mod that belongs to this module. It returns the directory containing
// that go.mod, or an error if none is found.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	for {
		if hasModuleFile(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(
				"could not find go.mod for %s in any parent directory",
				moduleName,
			)
		}
		dir = parent
	}
}

// hasModuleFile reports whether dir contains a go.mod whose first "module"
// directive matches moduleName.
func hasModuleFile(dir string) bool {
	f, err := os.Open(filepath.Clean(filepath.Join(dir, "go.mod")))
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(after) == moduleName
		}
	}
	return false
}
