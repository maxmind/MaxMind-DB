package writer

import (
	"bytes"
	"strings"
	"testing"
)

// TestWriteMapControlByte covers all three size forms. The fixtures only
// exercise the one-byte and three-byte forms, so a wrong byte in the two-byte
// form would otherwise reach a generated file unnoticed.
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
		name string
		call func()
	}{
		{"negative map size", func() { writeMap(make([]byte, 8), -1) }},
		{"oversized map", func() { writeMap(make([]byte, 8), maximumDataStructureSize+1) }},
		{"undersized large map", func() { writeLargeMap(make([]byte, 8), maximumSizeCode30) }},
		{"negative array size", func() { writeArrayHeader(make([]byte, 8), -1) }},
		{
			"oversized array",
			func() { writeArrayHeader(make([]byte, 8), maximumDataStructureSize+1) },
		},
		{"undersized large array", func() { writeLargeArray(make([]byte, 8), maximumSizeCode30) }},
		{"negative scalar size", func() { writeScalar(make([]byte, 8), -1, scalarTypeBytes) }},
		{"oversized scalar", func() {
			writeScalar(make([]byte, 8), maximumDataStructureSize+1, scalarTypeBytes)
		}},
		{"invalid scalar type", func() { writeScalar(make([]byte, 8), 0, 8) }},
		{"oversized search-tree record", func() {
			writeSearchTreeRecords(make([]byte, 8), maximum24BitSearchTreeValue+1, 0)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("call did not panic")
				}
			}()
			test.call()
		})
	}
}
