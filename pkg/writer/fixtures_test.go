package writer

import (
	"bytes"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

// TestFixedEpochFixturesMatchCommitted checks the mmdbwriter-generated
// databases that have a fixed build epoch. The other test-data databases record
// the build time. TestRawBadDataFixturesMatchCommitted checks the raw bad-data
// databases.
func TestFixedEpochFixturesMatchCommitted(t *testing.T) {
	dir := t.TempDir()
	w, err := New(filepath.Join("..", "..", "source-data"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteGeoIP2TestDB(); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBadDataDBs(dir); err != nil {
		t.Fatal(err)
	}

	for name, committed := range map[string]string{
		"GeoIP-Residential-Proxy-Test.mmdb":    "test-data",
		"libmaxminddb-deep-nesting.mmdb":       "bad-data/libmaxminddb",
		"libmaxminddb-deep-array-nesting.mmdb": "bad-data/libmaxminddb",
	} {
		//nolint:gosec // name comes from the fixed cases map, not user input.
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		//nolint:gosec // name comes from the fixed cases map, not user input.
		want, err := os.ReadFile(filepath.Join("..", "..", committed, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the committed file; regenerate with write-test-data", name)
		}
	}
}

func TestRawBadDataFixturesMatchCommitted(t *testing.T) {
	for name, got := range rawBadDataFixtures() {
		//nolint:gosec // name comes from the fixed fixtures map, not user input.
		want, err := os.ReadFile(filepath.Join("..", "..", "bad-data", "libmaxminddb", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the committed file; regenerate with write-test-data", name)
		}
	}
}

// TestRepeatedKeyFixturesRepeatKeys checks that each repeated-key fixture
// still repeats its key. The byte-equality test passes after a regeneration
// that drops the repeat, so it cannot catch that.
func TestRepeatedKeyFixturesRepeatKeys(t *testing.T) {
	tests := []struct {
		name        string
		description bool
		key         string
	}{
		{"libmaxminddb-repeated-metadata-key.mmdb", false, "database_type"},
		{"libmaxminddb-repeated-node-count.mmdb", false, "node_count"},
		{"libmaxminddb-repeated-pointer-key.mmdb", false, "database_type"},
		{"libmaxminddb-repeated-description-key.mmdb", true, "en"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(
				filepath.Join("..", "..", "bad-data", "libmaxminddb", test.name),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, metadata, found := bytes.CutLast(data, []byte(metadataMarker))
			if !found {
				t.Fatal("no metadata marker")
			}

			counts := countMapKeys(t, mmdbdata.NewDecoder(metadata, 0), test.description)
			if counts[test.key] != 2 {
				t.Errorf("key %q appears %d times, want 2", test.key, counts[test.key])
			}
		})
	}
}

// countMapKeys counts the keys of the top-level metadata map, or of its
// description map if description is true.
func countMapKeys(t *testing.T, decoder *mmdbdata.Decoder, description bool) map[string]int {
	t.Helper()
	keys, _, err := decoder.ReadMap()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for key, err := range keys {
		if err != nil {
			t.Fatal(err)
		}
		if description && string(key) == "description" {
			return countMapKeys(t, decoder, false)
		}
		counts[string(key)]++
	}
	if description {
		t.Fatal("no description key")
	}
	return counts
}

func TestDeepNestingFixturesExceedMaxDepth(t *testing.T) {
	for _, name := range []string{
		"libmaxminddb-deep-nesting.mmdb",
		"libmaxminddb-deep-array-nesting.mmdb",
	} {
		reader, err := maxminddb.Open(filepath.Join("..", "..", "bad-data", "libmaxminddb", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, ip := range []string{"1.1.1.1", "200.1.1.1"} {
			var v any
			err := reader.Lookup(netip.MustParseAddr(ip)).Decode(&v)
			if err == nil ||
				!strings.Contains(err.Error(), "exceeded maximum data structure depth") {
				t.Errorf("%s %s: error = %v, want a depth error", name, ip, err)
			}
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestMetadataPointersFixtureUsesPointers checks that the metadata stores its
// repeated string one time. The metadata uses the string four times, so the
// other uses must be pointers.
func TestMetadataPointersFixtureUsesPointers(t *testing.T) {
	data, err := os.ReadFile(
		filepath.Join("..", "..", "test-data", "MaxMind-DB-test-metadata-pointers.mmdb"),
	)
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.LastIndex(data, []byte("\xab\xcd\xefMaxMind.com"))
	if start < 0 {
		t.Fatal("metadata marker not found")
	}
	if n := bytes.Count(data[start:], []byte("Lots of pointers in metadata")); n != 1 {
		t.Errorf("metadata has %d copies of the repeated string, want 1", n)
	}
}
