package writer

import (
	"bytes"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2"
)

// countLeafDecodes walks the fan-out data section the way a reader without
// pointer memoization does, following every pointer and counting how many times
// the shared leaf is decoded. The leaf count is 2**depth and the total number
// of decode operations is Θ(2**depth), which makes the structure a denial of
// service.
func countLeafDecodes(data []byte, offset int) int {
	if data[offset]>>5 == 5 { // uint16 leaf
		return 1
	}
	// Array of size two: a two-byte header followed by two one-byte-payload
	// pointers. A pointer's value is ((byte0 & 0x7) << 8) | byte1.
	left := int(data[offset+2]&0x7)<<8 | int(data[offset+3])
	right := int(data[offset+4]&0x7)<<8 | int(data[offset+5])
	return countLeafDecodes(data, left) + countLeafDecodes(data, right)
}

func TestPointerFanOutDataIsExponential(t *testing.T) {
	for depth := 1; depth <= 10; depth++ {
		data, top := buildPointerFanOutData(depth)
		got := countLeafDecodes(data, top)
		want := 1 << depth
		if got != want {
			t.Errorf("depth %d: leaf decoded %d times, want %d", depth, got, want)
		}
	}
}

func TestPointerFanOutFixturesMatchCommitted(t *testing.T) {
	cases := map[string][]byte{
		pointerDoSFixtureFilename: buildPointerFanOutDB(pointerDoSDepth),
		pointerDoSIPv6FixtureFilename: buildPointerFanOutAllSpaceDB(
			pointerDoSDepth,
		),
	}
	for name, got := range cases {
		//nolint:gosec // name comes from the fixed cases map, not user input.
		want, err := os.ReadFile(filepath.Join("..", "..", "test-data", name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the committed file; regenerate with write-test-data", name)
		}
	}
}

func TestPointerFanOutFixturesAreSemanticallyValid(t *testing.T) {
	tests := []struct {
		name      string
		ipVersion uint
		nodeCount uint
		addresses []string
	}{
		{
			name:      pointerDoSFixtureFilename,
			ipVersion: 4,
			nodeCount: 1,
			addresses: []string{"0.0.0.0", "203.0.113.9", "255.255.255.255"},
		},
		{
			name:      pointerDoSIPv6FixtureFilename,
			ipVersion: 6,
			nodeCount: 97,
			addresses: []string{
				"0.0.0.0",
				"203.0.113.9",
				"255.255.255.255",
				"::",
				"2001:db8::1",
				"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Clean(filepath.Join("..", "..", "test-data", test.name))
			db, err := maxminddb.Open(path)
			if err != nil {
				t.Fatalf("opening fixture: %v", err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Errorf("closing fixture: %v", err)
				}
			})

			if got := db.Metadata.BinaryFormatMajorVersion; got != 2 {
				t.Errorf("binary format major version = %d, want 2", got)
			}
			if got := db.Metadata.BinaryFormatMinorVersion; got != 0 {
				t.Errorf("binary format minor version = %d, want 0", got)
			}
			if got := db.Metadata.BuildEpoch; got != pointerDoSBuildEpoch {
				t.Errorf("build epoch = %d, want %d", got, pointerDoSBuildEpoch)
			}
			if got := db.Metadata.DatabaseType; got != "Test" {
				t.Errorf("database type = %q, want %q", got, "Test")
			}
			if got := db.Metadata.IPVersion; got != test.ipVersion {
				t.Errorf("IP version = %d, want %d", got, test.ipVersion)
			}
			if got := db.Metadata.NodeCount; got != test.nodeCount {
				t.Errorf("node count = %d, want %d", got, test.nodeCount)
			}
			if got := db.Metadata.RecordSize; got != 24 {
				t.Errorf("record size = %d, want 24", got)
			}

			// Lookups stop at the outer record. Do not call Decode here: expanding
			// the fan-out is deliberately hostile in an unprotected reader.
			var outerOffset uintptr
			for i, address := range test.addresses {
				result := db.Lookup(netip.MustParseAddr(address))
				if err := result.Err(); err != nil {
					t.Errorf("looking up %s: %v", address, err)
					continue
				}
				if !result.Found() {
					t.Errorf("lookup for %s did not find a record", address)
					continue
				}
				if i == 0 {
					outerOffset = result.Offset()
				} else if got := result.Offset(); got != outerOffset {
					t.Errorf("lookup for %s returned offset %d, want %d", address, got, outerOffset)
				}
			}

			// The reader independently validates the metadata and search tree. Walk
			// only one branch per level to validate the data topology in bounded time.
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}
			recordSizeQuarter := uint64(db.Metadata.RecordSize / 4)
			if recordSizeQuarter == 0 ||
				uint64(
					db.Metadata.NodeCount,
				) > uint64(
					math.MaxInt-dataSeparatorSize,
				)/recordSizeQuarter {
				t.Fatalf(
					"search tree dimensions overflow int: %d nodes at %d bits per record",
					db.Metadata.NodeCount,
					db.Metadata.RecordSize,
				)
			}
			searchTreeSize64 := uint64(db.Metadata.NodeCount) * recordSizeQuarter
			//nolint:gosec // G115: searchTreeSize64 is checked against math.MaxInt above.
			searchTreeSize := int(searchTreeSize64)
			dataStart := searchTreeSize + dataSeparatorSize
			if dataStart > len(raw) {
				t.Fatalf("data section starts at %d, beyond file size %d", dataStart, len(raw))
			}
			if uint64(outerOffset) > uint64(math.MaxInt) {
				t.Fatalf("outer record offset %d overflows int", outerOffset)
			}
			outerOffsetInt := int(outerOffset)
			if outerOffsetInt >= len(raw)-dataStart {
				t.Fatalf("outer record offset %d is outside the data section", outerOffset)
			}
			verifyPointerFanOutRecord(t, raw[dataStart:], outerOffsetInt, pointerDoSDepth)
		})
	}
}

// verifyPointerFanOutRecord follows one of the two equal pointers at each
// level. Its work is linear in depth and never expands the hostile structure.
func verifyPointerFanOutRecord(t *testing.T, data []byte, offset, depth int) {
	t.Helper()
	for level := depth; level > 0; level-- {
		if offset < 0 || offset+6 > len(data) {
			t.Fatalf("level %d array at offset %d is outside the data section", level, offset)
		}
		if data[offset] != 0x02 || data[offset+1] != 0x04 {
			t.Fatalf("level %d at offset %d is not a two-element array", level, offset)
		}
		left := smallDataPointer(t, data, offset+2)
		right := smallDataPointer(t, data, offset+4)
		if left != right {
			t.Fatalf("level %d pointers differ: left %d, right %d", level, left, right)
		}
		if left >= offset {
			t.Fatalf(
				"level %d pointer target %d does not precede array offset %d",
				level,
				left,
				offset,
			)
		}
		offset = left
	}
	if offset != 0 {
		t.Fatalf("leaf offset = %d, want 0", offset)
	}
	if data[offset] != 0xA0 {
		t.Fatalf("leaf control byte = %#x, want 0xa0", data[offset])
	}
}

func smallDataPointer(t *testing.T, data []byte, offset int) int {
	t.Helper()
	if offset < 0 || offset+2 > len(data) {
		t.Fatalf("pointer at offset %d is outside the data section", offset)
	}
	control := data[offset]
	if control>>5 != 1 || control&0x18 != 0 {
		t.Fatalf("value at offset %d is not a two-byte data pointer", offset)
	}
	return int(control&0x7)<<8 | int(data[offset+1])
}
