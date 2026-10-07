package codec

import "testing"

func TestFragmentCodec(t *testing.T) {
	tests := []string{
		"top",
		"home",
		"about",
		"pricing",
		"comments",
		"section-2",
		"24",
		"some-random-fragment",
		"550e8400-e29b-41d4-a716-446655440000",
		"",
	}

	for _, expected := range tests {
		t.Run(expected, func(t *testing.T) {
			writer := NewBitWriter()

			if err := encodeFragment(writer, expected); err != nil {
				t.Fatalf(
					"encodeFragment() failed: %v",
					err,
				)
			}

			reader := NewBitReader(
				writer.Bytes(),
				writer.BitLen(),
			)

			actual, err := decodeFragment(reader)
			if err != nil {
				t.Fatalf(
					"decodeFragment() failed: %v",
					err,
				)
			}

			if actual != expected {
				t.Fatalf(
					"expected %q, got %q",
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
		})
	}
}

func TestCommonFragmentEncodingSize(t *testing.T) {
	tests := []string{
		"top",
		"home",
		"about",
		"pricing",
		"comments",
	}

	for _, fragment := range tests {
		t.Run(fragment, func(t *testing.T) {
			writer := NewBitWriter()

			if err := encodeFragment(writer, fragment); err != nil {
				t.Fatalf(
					"encodeFragment() failed: %v",
					err,
				)
			}

			// 1 bit marker + 3 bit dictionary token.
			const expectedBits uint64 = 4

			if writer.BitLen() != expectedBits {
				t.Fatalf(
					"expected %d bits, got %d",
					expectedBits,
					writer.BitLen(),
				)
			}
		})
	}
}

func TestRawFragmentEncodingSize(t *testing.T) {
	fragment := "section-2"

	writer := NewBitWriter()

	if err := encodeFragment(writer, fragment); err != nil {
		t.Fatalf(
			"encodeFragment() failed: %v",
			err,
		)
	}

	// 1 bit marker
	// 8 bits length
	// 9 × 8 bits data
	expectedBits := uint64(81)

	if writer.BitLen() != expectedBits {
		t.Fatalf(
			"expected %d bits, got %d",
			expectedBits,
			writer.BitLen(),
		)
	}
}
