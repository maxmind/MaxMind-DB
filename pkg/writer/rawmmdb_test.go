package writer

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestWriteMapControlByte covers the direct form and all three extended size
// forms. Current fixtures exercise the one- and three-byte map headers, so
// explicit cases preserve the two- and four-byte boundaries.
func TestWriteMapControlByte(t *testing.T) {
	tests := []struct {
		size int
		want []byte
	}{
		{0, []byte{0xE0}},
		{28, []byte{0xFC}},
		{29, []byte{0xFD, 0x00}},
		{284, []byte{0xFD, 0xFF}},
		{285, []byte{0xFE, 0x00, 0x00}},
		{512, []byte{0xFE, 0x00, 0xE3}},
		{65820, []byte{0xFE, 0xFF, 0xFF}},
		{65821, []byte{0xFF, 0x00, 0x00, 0x00}},
		{1_000_000, []byte{0xFF, 0x0E, 0x41, 0x23}},
		{maximumDataStructureSize, []byte{0xFF, 0xFF, 0xFF, 0xFF}},
	}
	for _, test := range tests {
		buf := make([]byte, 8)
		n := writeMap(buf, test.size)
		if got := buf[:n]; !bytes.Equal(got, test.want) {
			t.Errorf("writeMap(%d) = %#v, want %#v", test.size, got, test.want)
		}
	}
}

// TestWriteStringControlByte covers the boundary where the size no longer fits
// the control byte. Every current caller passes a short metadata key, so nothing
// else exercises the extended forms.
func TestWriteStringControlByte(t *testing.T) {
	tests := []struct {
		size       int
		wantHeader []byte
	}{
		{0, []byte{0x40}},
		{2, []byte{0x42}},
		{28, []byte{0x5C}},
		{29, []byte{0x5D, 0x00}},
		{284, []byte{0x5D, 0xFF}},
		{285, []byte{0x5E, 0x00, 0x00}},
		{65820, []byte{0x5E, 0xFF, 0xFF}},
		{65821, []byte{0x5F, 0x00, 0x00, 0x00}},
		{1_000_000, []byte{0x5F, 0x0E, 0x41, 0x23}},
	}
	for _, test := range tests {
		value := strings.Repeat("a", test.size)
		buf := make([]byte, test.size+8)
		n := writeString(buf, value)
		want := append(append([]byte{}, test.wantHeader...), value...)
		if got := buf[:n]; !bytes.Equal(got, want) {
			t.Errorf("writeString(%d bytes) = %#v, want %#v",
				test.size, got[:min(n, 4)], want[:min(len(want), 4)])
		}
	}
}

func TestWriteArrayHeader(t *testing.T) {
	tests := []struct {
		size int
		want []byte
	}{
		{0, []byte{0x00, 0x04}},
		{28, []byte{0x1C, 0x04}},
		{29, []byte{0x1D, 0x04, 0x00}},
		{284, []byte{0x1D, 0x04, 0xFF}},
		{285, []byte{0x1E, 0x04, 0x00, 0x00}},
		{65820, []byte{0x1E, 0x04, 0xFF, 0xFF}},
		{65821, []byte{0x1F, 0x04, 0x00, 0x00, 0x00}},
		{1_000_000, []byte{0x1F, 0x04, 0x0E, 0x41, 0x23}},
		{maximumDataStructureSize, []byte{0x1F, 0x04, 0xFF, 0xFF, 0xFF}},
	}
	for _, test := range tests {
		buf := make([]byte, 8)
		n := writeArrayHeader(buf, test.size)
		if got := buf[:n]; !bytes.Equal(got, test.want) {
			t.Errorf("writeArrayHeader(%d) = %#v, want %#v", test.size, got, test.want)
		}
	}
}

func TestWriteSearchTreeRecordsMaximum(t *testing.T) {
	buf := make([]byte, 6)
	n := writeSearchTreeRecords(buf, maximum24BitSearchTreeValue, maximum24BitSearchTreeValue)
	want := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if got := buf[:n]; !bytes.Equal(got, want) {
		t.Errorf("writeSearchTreeRecords() = %#v, want %#v", got, want)
	}
}

func TestRawWritersRejectInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		call      func()
		wantPanic string
	}{
		{
			"negative map size",
			func() { writeMap(make([]byte, 8), -1) },
			"map size -1 is outside the supported range",
		},
		{
			"oversized map",
			func() { writeMap(make([]byte, 8), maximumDataStructureSize+1) },
			"map size 16843037 is outside the supported range",
		},
		{
			"undersized large map",
			func() { writeLargeMap(make([]byte, 8), maximumSizeCode30) },
			"map size 65820 is outside the case-31 range",
		},
		{
			"oversized large map",
			func() { writeLargeMap(make([]byte, 8), maximumDataStructureSize+1) },
			"map size 16843037 is outside the case-31 range",
		},
		{
			"negative array size",
			func() { writeArrayHeader(make([]byte, 8), -1) },
			"array size -1 is outside the supported range",
		},
		{
			"oversized array",
			func() { writeArrayHeader(make([]byte, 8), maximumDataStructureSize+1) },
			"array size 16843037 is outside the supported range",
		},
		{
			"undersized large array",
			func() { writeLargeArray(make([]byte, 8), maximumSizeCode30) },
			"array size 65820 is outside the case-31 range",
		},
		{
			"oversized large array",
			func() { writeLargeArray(make([]byte, 8), maximumDataStructureSize+1) },
			"array size 16843037 is outside the case-31 range",
		},
		{
			"negative scalar size",
			func() { writeScalar(make([]byte, 8), -1, scalarTypeBytes) },
			"scalar size -1 is outside the supported range",
		},
		{
			"oversized scalar",
			func() { writeScalar(make([]byte, 8), maximumDataStructureSize+1, scalarTypeBytes) },
			"scalar size 16843037 is outside the supported range",
		},
		{
			"invalid scalar type",
			func() { writeScalar(make([]byte, 8), 0, 8) },
			"unsupported scalar type 8",
		},
		{
			"oversized left search-tree record",
			func() {
				writeSearchTreeRecords(make([]byte, 8), maximum24BitSearchTreeValue+1, 0)
			},
			"search-tree record values 16777216 and 0 must fit in 24 bits",
		},
		{
			"oversized right search-tree record",
			func() {
				writeSearchTreeRecords(make([]byte, 8), 0, maximum24BitSearchTreeValue+1)
			},
			"search-tree record values 0 and 16777216 must fit in 24 bits",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				got := recover()
				if got == nil {
					t.Error("call did not panic")
				} else if message := fmt.Sprint(got); !strings.Contains(message, test.wantPanic) {
					t.Errorf("panic = %q, want it to contain %q", message, test.wantPanic)
				}
			}()
			test.call()
		})
	}
}
