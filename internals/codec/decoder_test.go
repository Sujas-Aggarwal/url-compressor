package codec

import "testing"

func TestDecode(t *testing.T) {
	tests := []string{
		"https://example.com",
		"https://www.example.com",
		"http://example.com",
		"http://example.com:8080",
		"https://example.com/",
		"https://example.com/products",
		"https://example.com/products/phones",
		"https://www.youtube.com/watch?v=123",
		"https://www.youtube.com/watch?v=123&page=2",
		"https://example.com/search?q=golang&page=2",
		"https://example.com/products/iphone-17?color=black#reviews",
		"https://www.flipkart.com:9000/hindi-crossword-puzzle-book/p/itm8cd1269c479c4?potato=23&tomato=23#24",
		"http://localhost:3000/api",
		"ftp://example.com/file.txt",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			data, bitLen, err := Encode(input)
			if err != nil {
				t.Fatalf("Encode() failed: %v", err)
			}

			output, err := Decode(data, bitLen)
			if err != nil {
				t.Fatalf("Decode() failed: %v", err)
			}

			if output != input {
				t.Fatalf(
					"round trip failed:\nexpected: %q\ngot:      %q",
					input,
					output,
				)
			}
		})
	}
}

func TestDecodeInvalidBitLength(t *testing.T) {
	data, bitLen, err := Encode("https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	tests := []uint64{
		0,
		bitLen + 1,
		uint64(len(data))*8 + 1,
	}

	for _, invalidBitLen := range tests {
		t.Run(
			"invalid",
			func(t *testing.T) {
				if _, err := Decode(data, invalidBitLen); err == nil {
					t.Fatalf(
						"expected error for bit length %d",
						invalidBitLen,
					)
				}
			},
		)
	}
}

func TestDecodeUnsupportedVersion(t *testing.T) {
	writer := NewBitWriter()

	// Version 1.
	if err := writer.WriteBits(1, 3); err != nil {
		t.Fatal(err)
	}

	// Flags.
	if err := writer.WriteBits(0, 8); err != nil {
		t.Fatal(err)
	}

	_, err := Decode(writer.Bytes(), writer.BitLen())
	if err == nil {
		t.Fatal("expected unsupported version error")
	}
}

func TestEncodeDecodeAllComponents(t *testing.T) {
	input := "https://www.example.com:9000/api/v1/users/123?query=golang&page=2#comments"

	data, bitLen, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode() failed: %v", err)
	}

	output, err := Decode(data, bitLen)
	if err != nil {
		t.Fatalf("Decode() failed: %v", err)
	}

	if output != input {
		t.Fatalf(
			"round trip failed:\nexpected: %q\ngot:      %q",
			input,
			output,
		)
	}
}

func TestDecodeDoesNotAcceptTrailingBits(t *testing.T) {
	input := "https://example.com"

	data, bitLen, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}

	// Add one extra zero bit to the declared payload.
	_, err = Decode(data, bitLen+1)
	if err == nil {
		t.Fatal("expected trailing-data error")
	}
}
