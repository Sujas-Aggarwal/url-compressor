package codec

import "testing"

func TestSemanticRoundTrip(t *testing.T) {
	tests := []string{
		"https://google.com",
		"https://www.google.com",
		"http://google.com:8080/test",
		"https://www.flipkart.com:9000/hindi-crossword-puzzle-book/p/itm8cd1269c479c4?potato=23&tomato=23#24",
		"ftp://www.example.com/file.txt",
		"mailto:test@example.com",
	}

	for _, input := range tests {
		parsed, err := parse(input)
		if err != nil {
			t.Fatalf("parse failed for %q: %v", input, err)
		}

		encoded := encodeSemantic(parsed)
		decoded := decodeSemantic(encoded)

		// Reconstruct a URL for comparison.
		if decoded.Scheme != parsed.Scheme {
			t.Errorf("%q: scheme mismatch", input)
		}

		if decoded.Host != parsed.Host {
			t.Errorf("%q: host mismatch: got %q want %q",
				input, decoded.Host, parsed.Host)
		}

		if decoded.Port != parsed.Port {
			t.Errorf("%q: port mismatch", input)
		}

		if decoded.Path != parsed.Path {
			t.Errorf("%q: path mismatch", input)
		}

		if decoded.Query != parsed.Query {
			t.Errorf("%q: query mismatch", input)
		}

		if decoded.Fragment != parsed.Fragment {
			t.Errorf("%q: fragment mismatch", input)
		}
	}
}
