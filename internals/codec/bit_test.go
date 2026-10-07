package codec

import "testing"

func TestBitWriterReader(t *testing.T) {
	writer := NewBitWriter()

	tests := []struct {
		value uint64
		bits  uint8
	}{
		{1, 1},
		{5, 3},
		{255, 8},
		{17, 5},
		{123456, 20},
		{0xDEADBEEF, 32},
	}

	for _, tc := range tests {
		if err := writer.WriteBits(tc.value, tc.bits); err != nil {
			t.Fatalf("WriteBits failed: %v", err)
		}
	}

	reader := NewBitReader(writer.Bytes(), writer.BitLen())

	for _, tc := range tests {
		got, err := reader.ReadBits(tc.bits)
		if err != nil {
			t.Fatalf("ReadBits failed: %v", err)
		}

		expected := tc.value

		if tc.bits < 64 {
			expected &= (uint64(1) << tc.bits) - 1
		}

		if got != expected {
			t.Fatalf(
				"expected %d, got %d (%d bits)",
				expected,
				got,
				tc.bits,
			)
		}
	}

	if reader.Remaining() != 0 {
		t.Fatalf("expected 0 remaining bits, got %d", reader.Remaining())
	}
}

func TestBitWriterByteBoundary(t *testing.T) {
	writer := NewBitWriter()

	if err := writer.WriteBits(0b101, 3); err != nil {
		t.Fatal(err)
	}

	if err := writer.WriteBits(0b11001, 5); err != nil {
		t.Fatal(err)
	}

	if writer.BitLen() != 8 {
		t.Fatalf("expected 8 bits, got %d", writer.BitLen())
	}

	data := writer.Bytes()

	if len(data) != 1 {
		t.Fatalf("expected 1 byte, got %d", len(data))
	}

	if data[0] != 0b10111001 {
		t.Fatalf("expected 10111001, got %08b", data[0])
	}
}

func TestBitWriterPartialByte(t *testing.T) {
	writer := NewBitWriter()

	if err := writer.WriteBits(0b101, 3); err != nil {
		t.Fatal(err)
	}

	data := writer.Bytes()

	if len(data) != 1 {
		t.Fatalf("expected 1 byte, got %d", len(data))
	}

	// 101 followed by five zero-padding bits.
	if data[0] != 0b10100000 {
		t.Fatalf("expected 10100000, got %08b", data[0])
	}

	reader := NewBitReader(data, writer.BitLen())

	value, err := reader.ReadBits(3)
	if err != nil {
		t.Fatal(err)
	}

	if value != 0b101 {
		t.Fatalf("expected 101, got %03b", value)
	}

	if reader.Remaining() != 0 {
		t.Fatalf("expected 0 remaining bits, got %d", reader.Remaining())
	}
}

func TestBitReaderRejectsOverflow(t *testing.T) {
	writer := NewBitWriter()

	if err := writer.WriteBits(0b101, 3); err != nil {
		t.Fatal(err)
	}

	reader := NewBitReader(writer.Bytes(), writer.BitLen())

	_, err := reader.ReadBits(4)

	if err == nil {
		t.Fatal("expected error when reading beyond bitstream")
	}
}
