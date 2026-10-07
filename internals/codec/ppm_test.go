package codec

import (
	"bytes"
	"testing"
)

func TestPPMEncodeDecode(t *testing.T) {
	training := []byte(
		"the quick brown fox jumps over the lazy dog " +
			"the quick brown fox jumps over the lazy dog " +
			"the quick brown fox jumps over the lazy dog ",
	)

	model := NewPPMModel()

	// Train the adaptive model.
	for i := 0; i < 100; i++ {
		model.Update(training)
	}

	input := []byte(
		"the quick brown fox jumps over the lazy dog",
	)

	encoded, err := model.Encode(input)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := model.Decode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !bytes.Equal(input, decoded) {
		t.Fatalf(
			"round trip mismatch:\nwant: %q\ngot:  %q",
			input,
			decoded,
		)
	}
}

func TestPPMShortInputs(t *testing.T) {
	tests := [][]byte{
		[]byte("a"),
		[]byte("ab"),
		[]byte("hello"),
		[]byte("https://example.com"),
		[]byte("/users/123/products/456"),
	}

	for _, input := range tests {
		t.Run(string(input), func(t *testing.T) {
			model := NewPPMModel()

			for i := 0; i < 20; i++ {
				model.Update(input)
			}

			encoded, err := model.Encode(input)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			decoded, err := model.Decode(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			if !bytes.Equal(input, decoded) {
				t.Fatalf(
					"want %q, got %q",
					input,
					decoded,
				)
			}
		})
	}
}

func TestPPMRepeatedPattern(t *testing.T) {
	input := []byte(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)

	model := NewPPMModel()

	for i := 0; i < 100; i++ {
		model.Update(input)
	}

	encoded, err := model.Encode(input)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := model.Decode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !bytes.Equal(input, decoded) {
		t.Fatalf("round trip failed")
	}

	originalBits := len(input) * 8
	encodedBits := len(encoded) * 8

	if encodedBits >= originalBits {
		t.Fatalf(
			"expected compression: original=%d bits encoded=%d bits",
			originalBits,
			encodedBits,
		)
	}
}

func TestPPMContextOrder(t *testing.T) {
	model := NewPPMModel()

	input := []byte("abcdefghijklmnop")

	for i := 0; i < 10; i++ {
		model.Update(input)
	}

	if len(model.orders[1]) == 0 {
		t.Fatal("order-1 model is empty")
	}

	if len(model.orders[2]) == 0 {
		t.Fatal("order-2 model is empty")
	}

	if len(model.orders[3]) == 0 {
		t.Fatal("order-3 model is empty")
	}

	if len(model.orders[4]) == 0 {
		t.Fatal("order-4 model is empty")
	}
}
