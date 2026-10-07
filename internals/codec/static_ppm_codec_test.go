package codec

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticPPMCodecRoundTrip(t *testing.T) {
	model := NewStaticPPM()
	model.MaxOrder = 2

	// Order 0.
	model.contexts[staticPPMContextKey(0, 0)] = &StaticPPMContext{
		Key:   0,
		Order: 0,
		Total: 10,
		Symbols: []StaticPPMSymbol{
			{Symbol: 'a', Count: 5},
			{Symbol: 'b', Count: 3},
			{Symbol: 'c', Count: 2},
		},
	}

	// Order 1.
	model.contexts[staticPPMContextKey(1, 'a')] = &StaticPPMContext{
		Key:   uint32('a'),
		Order: 1,
		Total: 8,
		Symbols: []StaticPPMSymbol{
			{Symbol: 'b', Count: 5},
			{Symbol: 'a', Count: 3},
		},
	}

	// Order 2: "ab".
	model.contexts[staticPPMContextKey(2, uint32('a')<<8|uint32('b'))] = &StaticPPMContext{
		Key:   uint32('a')<<8 | uint32('b'),
		Order: 2,
		Total: 6,
		Symbols: []StaticPPMSymbol{
			{Symbol: 'c', Count: 4},
			{Symbol: 'a', Count: 2},
		},
	}

	encoder := NewStaticPPMEncoder(model)
	decoder := NewStaticPPMDecoder(model)

	tests := [][]byte{
		[]byte("abc"),
		[]byte("abababab"),
		[]byte("aaaaabbbbb"),
		[]byte("xyz"),
		[]byte("a\xffc"),
	}

	for _, input := range tests {
		input := input

		t.Run(string(input), func(t *testing.T) {
			encoded, err := encoder.Encode(input)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			decoded, err := decoder.Decode(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			if !bytes.Equal(decoded, input) {
				t.Fatalf(
					"round trip mismatch:\ninput:   %v\ndecoded: %v",
					input,
					decoded,
				)
			}
		})
	}
}

func TestStaticPPMRealModelRoundTrip(t *testing.T) {
	path := filepath.Join("../../", "corpus", "model", "ppm3.bin")

	if _, err := os.Stat(path); err != nil {
		t.Skipf("trained model not available: %v", err)
	}

	model, err := LoadStaticPPM(path)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}

	encoder := NewStaticPPMEncoder(model)
	decoder := NewStaticPPMDecoder(model)

	tests := []string{
		"https://www.google.com/",
		"https://www.flipkart.com/search?q=iphone",
		"https://example.com/a/b/c?foo=bar&baz=123",
		"https://github.com/Sujas-Aggarwal/url-compressor",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://example.com/path/with/a/very/long/string?query=hello%20world",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			encoded, err := encoder.Encode([]byte(input))
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			decoded, err := decoder.Decode(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			if string(decoded) != input {
				t.Fatalf(
					"round trip mismatch:\ninput:   %q\ndecoded: %q",
					input,
					decoded,
				)
			}

			t.Logf(
				"original=%d bytes compressed=%d bytes ratio=%.3f",
				len(input),
				len(encoded),
				float64(len(encoded))/float64(len(input)),
			)
		})
	}
}
