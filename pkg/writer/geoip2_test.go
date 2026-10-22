package writer

import (
	"encoding/json/v2"
	"fmt"
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

// TestNamesAgreeForEachGeoNameID checks that each place with names has a
// geoname_id, and that every place with the same geoname_id has the same
// names in all source data files. A CSV location file has one row for each
// geoname_id, so conflicting names make that row depend on network order.
func TestNamesAgreeForEachGeoNameID(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "source-data", "*.json"))
	if err != nil {
		t.Fatal(err)
	}

	names := map[float64]any{}
	where := map[float64]string{}
	for _, path := range paths {
		name := filepath.Base(path)
		for prefix, record := range readSourceData(t, name) {
			checkNames(t, record, name+" "+prefix.String(), names, where)
		}
	}
}

// TestCountryHasContinent checks that each GeoIP2 and GeoLite2 record with a
// country also has a continent, as the production databases do.
func TestCountryHasContinent(t *testing.T) {
	var paths []string
	for _, pattern := range []string{"GeoIP2-*.json", "GeoLite2-*.json"} {
		matches, err := filepath.Glob(filepath.Join("..", "..", "source-data", pattern))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}

	for _, path := range paths {
		name := filepath.Base(path)
		for prefix, record := range readSourceData(t, name) {
			_, hasCountry := record["country"]
			_, hasContinent := record["continent"]
			if hasCountry && !hasContinent {
				t.Errorf("%s %s has a country but no continent", name, prefix)
			}
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

// checkNames walks value and checks each place that has names against the
// first names seen for its geoname_id. names and where record those first
// names and their location.
func checkNames(
	t *testing.T,
	value any,
	location string,
	names map[float64]any,
	where map[float64]string,
) {
	t.Helper()

	switch v := value.(type) {
	case map[string]any:
		if placeNames, ok := v["names"]; ok {
			id, ok := v["geoname_id"].(float64)
			switch {
			case !ok:
				t.Errorf("%s has names but no geoname_id", location)
			case names[id] == nil:
				names[id] = placeNames
				where[id] = location
			case !reflect.DeepEqual(placeNames, names[id]):
				t.Errorf(
					"%s names for geoname_id %v = %v, want the names at %s: %v",
					location, id, placeNames, where[id], names[id],
				)
			default:
				// The names match the first names for this geoname_id.
			}
		}
		for key, field := range v {
			checkNames(t, field, location+"."+key, names, where)
		}
	case []any:
		for i, item := range v {
			checkNames(t, item, fmt.Sprintf("%s[%d]", location, i), names, where)
		}
	default:
		// A scalar holds no names.
	}
}
