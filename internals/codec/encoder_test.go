package codec

import "testing"

func TestEncode(t *testing.T) {
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

			if len(data) == 0 {
				t.Fatal("Encode() returned empty data")
			}

			if bitLen == 0 {
				t.Fatal("Encode() returned zero bits")
			}

			if bitLen > uint64(len(data)*8) {
				t.Fatalf(
					"invalid bit length: bitLen=%d bytes=%d",
					bitLen,
					len(data),
				)
			}

			t.Logf(
				"encoded %d bits (%d bytes)",
				bitLen,
				len(data),
			)
		})
	}
}

func TestEncodeCompressionSize(t *testing.T) {
	tests := []string{
		"https://www.youtube.com/watch?v=123&page=2#comments",
		"https://www.flipkart.com:9000/hindi-crossword-puzzle-book/p/itm8cd1269c479c4?potato=23&tomato=23#24",
		"https://example.com/products/iphone-17?color=black",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			data, bitLen, err := Encode(input)
			if err != nil {
				t.Fatalf("Encode() failed: %v", err)
			}

			rawBits := uint64(len(input) * 8)

			t.Logf("URL:     %s", input)
			t.Logf("raw:     %d bits (%d bytes)", rawBits, len(input))
			t.Logf("encoded: %d bits (%d bytes)", bitLen, len(data))
			t.Logf("ratio:   %.3f", float64(bitLen)/float64(rawBits))
		})
	}
}

func TestEncodeDeterministic(t *testing.T) {
	input := "https://www.youtube.com/watch?v=123&page=2#comments"

	data1, bitLen1, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}

	data2, bitLen2, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}

	if bitLen1 != bitLen2 {
		t.Fatalf(
			"bit length changed: first=%d second=%d",
			bitLen1,
			bitLen2,
		)
	}

	if string(data1) != string(data2) {
		t.Fatal("encoding is not deterministic")
	}
}
