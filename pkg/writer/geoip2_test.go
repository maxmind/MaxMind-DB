package writer

import (
	"encoding/json/v2"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/maxmind/mmdbwriter/v2"
)

func TestInsertJSONRejectsDuplicateNetwork(t *testing.T) {
	// The two keys differ only in host bits, so they are the same network.
	path := filepath.Join(t.TempDir(), "dup.json")
	data := `[{"1.1.1.0/24": {"a": "x"}}, {"1.1.1.1/24": {"a": "y"}}]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	tree, err := mmdbwriter.New(mmdbwriter.Options{IPVersion: 4, RecordSize: 24})
	if err != nil {
		t.Fatal(err)
	}

	err = InsertJSON(tree, path)
	if err == nil || !strings.Contains(err.Error(), "duplicate network") {
		t.Fatalf("InsertJSON error = %v, want a duplicate network error", err)
	}
}

func TestInsertJSONRejectsIPv4InIPv6(t *testing.T) {
	for _, network := range []string{"::1.1.1.0/120", "::ffff:1.1.1.0/120"} {
		path := filepath.Join(t.TempDir(), "ipv4.json")
		data := `[{"` + network + `": {"a": "x"}}]`
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}

		tree, err := mmdbwriter.New(mmdbwriter.Options{IPVersion: 6, RecordSize: 24})
		if err != nil {
			t.Fatal(err)
		}

		err = InsertJSON(tree, path)
		if err == nil || !strings.Contains(err.Error(), "write it as 1.1.1.0/24") {
			t.Errorf("%s: InsertJSON error = %v, want an IPv4 form error", network, err)
		}
	}
}

// TestCountryMatchesEnterprise checks that a Country network contains each
// Enterprise network with country-level data, and that every Country network
// that overlaps it has the same country-level data. An IP address then gives
// the same country-level answer in both databases.
func TestCountryMatchesEnterprise(t *testing.T) {
	country := readSourceData(t, "GeoIP2-Country-Test.json")
	enterprise := readSourceData(t, "GeoIP2-Enterprise-Test.json")

	for ePrefix, eRecord := range enterprise {
		want := countryLevelData(t, eRecord)
		if len(want) == 0 {
			continue
		}
		contained := false
		for cPrefix, cRecord := range country {
			if !cPrefix.Overlaps(ePrefix) {
				continue
			}
			if got := countryLevelData(t, cRecord); !reflect.DeepEqual(got, want) {
				t.Errorf(
					"Country network %s data = %v, want the data of Enterprise network %s: %v",
					cPrefix, got, ePrefix, want,
				)
			}
			if cPrefix.Bits() <= ePrefix.Bits() {
				contained = true
			}
		}
		if !contained {
			t.Errorf("no Country network contains Enterprise network %s", ePrefix)
		}
	}
}

// readSourceData returns the records of a source data file by network.
func readSourceData(t *testing.T, name string) map[netip.Prefix]map[string]any {
	t.Helper()

	//nolint:gosec // name is a fixed source data file name, not user input.
	file, err := os.Open(filepath.Join("..", "..", "source-data", name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var data []map[string]map[string]any
	if err := json.UnmarshalRead(file, &data); err != nil {
		t.Fatal(err)
	}

	records := map[netip.Prefix]map[string]any{}
	for _, entry := range data {
		for network, record := range entry {
			prefix, err := netip.ParsePrefix(network)
			if err != nil {
				t.Fatal(err)
			}
			records[prefix.Masked()] = record
		}
	}
	return records
}

// countryLevelData returns the fields of record that a Country database has.
// It removes the confidence values, which only Enterprise has.
func countryLevelData(t *testing.T, record map[string]any) map[string]any {
	t.Helper()

	data := map[string]any{}
	for _, key := range []string{
		"continent",
		"country",
		"registered_country",
		"represented_country",
	} {
		value, ok := record[key]
		if !ok {
			continue
		}
		fields, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s is a %T, want an object", key, value)
		}
		fields = maps.Clone(fields)
		delete(fields, "confidence")
		data[key] = fields
	}

	if traits, ok := record["traits"].(map[string]any); ok {
		if anycast, ok := traits["is_anycast"]; ok {
			data["traits"] = map[string]any{"is_anycast": anycast}
		}
	}
	return data
}
