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
		t.Run(input, func(t *testing.T) {
			parsed, err := parse(input)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			encoded := encodeSemantic(parsed)
			decoded := decodeSemantic(encoded)

			if decoded.Scheme != parsed.Scheme {
				t.Errorf(
					"scheme mismatch: got %q want %q",
					decoded.Scheme,
					parsed.Scheme,
				)
			}

			if decoded.Host != parsed.Host {
				t.Errorf(
					"host mismatch: got %q want %q",
					decoded.Host,
					parsed.Host,
				)
			}

			if decoded.Port != parsed.Port {
				t.Errorf(
					"port mismatch: got %q want %q",
					decoded.Port,
					parsed.Port,
				)
			}

			if decoded.Path != parsed.Path {
				t.Errorf(
					"path mismatch: got %q want %q",
					decoded.Path,
					parsed.Path,
				)
			}

			if decoded.Query != parsed.Query {
				t.Errorf(
					"query mismatch: got %q want %q",
					decoded.Query,
					parsed.Query,
				)
			}

			if decoded.Fragment != parsed.Fragment {
				t.Errorf(
					"fragment mismatch: got %q want %q",
					decoded.Fragment,
					parsed.Fragment,
				)
			}
		})
	}
}

func TestSemanticFlags(t *testing.T) {
	tests := []struct {
		name string
		url  string
		flag Flags
	}{
		{
			name: "www",
			url:  "https://www.google.com",
			flag: FlagHasWWW,
		},
		{
			name: "port",
			url:  "https://google.com:8080",
			flag: FlagHasPort,
		},
		{
			name: "path",
			url:  "https://google.com/test",
			flag: FlagHasPath,
		},
		{
			name: "query",
			url:  "https://google.com?x=1",
			flag: FlagHasQuery,
		},
		{
			name: "fragment",
			url:  "https://google.com#test",
			flag: FlagHasFragment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parse(tt.url)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			encoded := encodeSemantic(parsed)

			if encoded.Flags&tt.flag == 0 {
				t.Errorf(
					"expected flag %v to be set for %q",
					tt.flag,
					tt.url,
				)
			}
		})
	}
}

func TestSemanticWWWRemoval(t *testing.T) {
	parsed, err := parse("https://www.google.com")
	if err != nil {
		t.Fatal(err)
	}

	encoded := encodeSemantic(parsed)

	if encoded.Host != "google.com" {
		t.Fatalf(
			"expected semantic host to remove www.: got %q",
			encoded.Host,
		)
	}

	if encoded.Flags&FlagHasWWW == 0 {
		t.Fatal("expected FlagHasWWW to be set")
	}

	decoded := decodeSemantic(encoded)

	if decoded.Host != "www.google.com" {
		t.Fatalf(
			"expected decoded host to restore www.: got %q",
			decoded.Host,
		)
	}
}
