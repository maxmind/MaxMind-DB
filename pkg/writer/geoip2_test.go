package writer

import (
	"os"
	"path/filepath"
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
