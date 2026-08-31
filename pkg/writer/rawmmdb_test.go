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
	}
	for _, test := range tests {
		value := strings.Repeat("a", test.size)
		buf := make([]byte, 1024)
		n := writeString(buf, value)
		want := append(append([]byte{}, test.wantHeader...), value...)
		if got := buf[:n]; !bytes.Equal(got, want) {
			t.Errorf("writeString(%d bytes) = %#v, want %#v",
				test.size, got[:min(n, 4)], want[:min(len(want), 4)])
		}
	}
}

func TestWriteMapRejectsOversize(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("writeMap(65821) did not panic")
		}
	}()
	writeMap(make([]byte, 8), 65821)
}
