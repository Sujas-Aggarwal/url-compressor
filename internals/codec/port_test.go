package codec

import "testing"

func TestPortCodec(t *testing.T) {
	tests := []uint16{
		80,
		443,
		8080,
		8443,
		3000,
		5000,
		8000,
		9000,
		54321,
		1,
		65535,
		0,
	}

	for _, expected := range tests {
		t.Run(
			string(rune(expected)),
			func(t *testing.T) {
				writer := NewBitWriter()

				if err := encodePort(writer, expected); err != nil {
					t.Fatalf(
						"encodePort(%d) failed: %v",
						expected,
						err,
					)
				}

				reader := NewBitReader(
					writer.Bytes(),
					writer.BitLen(),
				)

				actual, err := decodePort(reader)
				if err != nil {
					t.Fatalf(
						"decodePort() failed for port %d: %v",
						expected,
						err,
					)
				}

				if actual != expected {
					t.Fatalf(
						"expected port %d, got %d",
						expected,
						actual,
					)
				}

				if reader.Remaining() != 0 {
					t.Fatalf(
						"expected 0 remaining bits, got %d",
						reader.Remaining(),
					)
				}
			},
		)
	}
}

func TestCommonPortEncodingSize(t *testing.T) {
	tests := []struct {
		port uint16
		bits uint64
	}{
		{80, 4},
		{443, 4},
		{8080, 4},
		{8443, 4},
		{3000, 4},
		{5000, 4},
		{8000, 4},
		{9000, 4},
	}

	for _, tc := range tests {
		t.Run(
			string(rune(tc.port)),
			func(t *testing.T) {
				writer := NewBitWriter()

				if err := encodePort(writer, tc.port); err != nil {
					t.Fatalf("encodePort() failed: %v", err)
				}

				if writer.BitLen() != tc.bits {
					t.Fatalf(
						"port %d: expected %d bits, got %d",
						tc.port,
						tc.bits,
						writer.BitLen(),
					)
				}
			},
		)
	}
}

func TestUnknownPortEncodingSize(t *testing.T) {
	tests := []uint16{
		1,
		22,
		54321,
		65535,
	}

	for _, port := range tests {
		t.Run(
			string(rune(port)),
			func(t *testing.T) {
				writer := NewBitWriter()

				if err := encodePort(writer, port); err != nil {
					t.Fatalf("encodePort() failed: %v", err)
				}

				// 1 bit raw marker + 16 bits for the port.
				expectedBits := uint64(17)

				if writer.BitLen() != expectedBits {
					t.Fatalf(
						"port %d: expected %d bits, got %d",
						port,
						expectedBits,
						writer.BitLen(),
					)
				}
			},
		)
	}
}

func TestPortCodecAllValues(t *testing.T) {
	// Exhaustively verify the uint16 range.
	//
	// This catches issues around:
	// - 0
	// - 65535
	// - dictionary boundaries
	// - uint16 conversion
	// - bitstream boundaries
	for port := 0; port <= 65535; port++ {
		expected := uint16(port)

		writer := NewBitWriter()

		if err := encodePort(writer, expected); err != nil {
			t.Fatalf(
				"encodePort(%d) failed: %v",
				expected,
				err,
			)
		}

		reader := NewBitReader(
			writer.Bytes(),
			writer.BitLen(),
		)

		actual, err := decodePort(reader)
		if err != nil {
			t.Fatalf(
				"decodePort(%d) failed: %v",
				expected,
				err,
			)
		}

		if actual != expected {
			t.Fatalf(
				"expected %d, got %d",
				expected,
				actual,
			)
		}

		if reader.Remaining() != 0 {
			t.Fatalf(
				"port %d: expected 0 remaining bits, got %d",
				expected,
				reader.Remaining(),
			)
		}
	}
}
