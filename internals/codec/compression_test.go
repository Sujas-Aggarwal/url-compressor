package codec

import "testing"

func TestCompressionSize(t *testing.T) {
	input := "https://www.flipkart.com:9000/hindi-crossword-puzzle-book/p/itm8cd1269c479c4?potato=23&tomato=23#24"

	data, bitLen, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}

	rawBits := len(input) * 8

	t.Logf("URL:       %s", input)
	t.Logf("raw:       %d bits (%d bytes)", rawBits, len(input))
	t.Logf("encoded:   %d bits (%d bytes)", bitLen, len(data))
	t.Logf("ratio:     %.3f", float64(bitLen)/float64(rawBits))
}
