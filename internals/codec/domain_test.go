package codec

import "testing"

func TestParseDomain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple domain",
			input:    "example.com",
			expected: []string{"example", "com"},
		},
		{
			name:     "www domain",
			input:    "www.youtube.com",
			expected: []string{"www", "youtube", "com"},
		},
		{
			name:     "multi-level domain",
			input:    "api.v2.example.co.uk",
			expected: []string{"api", "v2", "example", "co", "uk"},
		},
		{
			name:     "localhost",
			input:    "localhost",
			expected: []string{"localhost"},
		},
		{
			name:     "subdomain",
			input:    "cdn.assets.example.com",
			expected: []string{"cdn", "assets", "example", "com"},
		},
		{
			name:     "trailing dot",
			input:    "example.com.",
			expected: []string{"example", "com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			domain, err := parseDomain(tc.input)
			if err != nil {
				t.Fatalf("parseDomain() error = %v", err)
			}

			if len(domain.Labels) != len(tc.expected) {
				t.Fatalf(
					"expected %d labels, got %d",
					len(tc.expected),
					len(domain.Labels),
				)
			}

			for i, expected := range tc.expected {
				if domain.Labels[i] != expected {
					t.Errorf(
						"label %d: expected %q, got %q",
						i,
						expected,
						domain.Labels[i],
					)
				}
			}
		})
	}
}

func TestDomainString(t *testing.T) {
	tests := []string{
		"example.com",
		"www.youtube.com",
		"api.v2.example.co.uk",
		"localhost",
		"cdn.assets.example.com",
	}

	for _, expected := range tests {
		t.Run(expected, func(t *testing.T) {
			domain, err := parseDomain(expected)
			if err != nil {
				t.Fatalf("parseDomain() error = %v", err)
			}

			got := domain.String()

			if got != expected {
				t.Fatalf(
					"expected %q, got %q",
					expected,
					got,
				)
			}
		})
	}
}

func TestParseDomainEmpty(t *testing.T) {
	domain, err := parseDomain("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(domain.Labels) != 0 {
		t.Fatalf(
			"expected 0 labels, got %d",
			len(domain.Labels),
		)
	}

	if domain.String() != "" {
		t.Fatalf(
			"expected empty string, got %q",
			domain.String(),
		)
	}
}

func TestParseDomainInvalidEmptyLabel(t *testing.T) {
	tests := []string{
		"example..com",
		".example.com",
		"example.com..",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseDomain(input)

			if err == nil {
				t.Fatalf(
					"expected error for invalid domain %q",
					input,
				)
			}
		})
	}
}

func TestParseDomainLabelTooLong(t *testing.T) {
	// DNS labels are limited to 63 octets.
	longLabel := ""

	for i := 0; i < 64; i++ {
		longLabel += "a"
	}

	input := longLabel + ".com"

	_, err := parseDomain(input)

	if err == nil {
		t.Fatal("expected error for label longer than 63 characters")
	}
}

func TestDomainCodec(t *testing.T) {
	tests := []string{
		"example.com",
		"www.youtube.com",
		"api.v2.example.co.uk",
		"localhost",
		"cdn.assets.example.com",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			domain, err := parseDomain(input)
			if err != nil {
				t.Fatalf("parseDomain() error = %v", err)
			}

			writer := NewBitWriter()

			if err := encodeDomain(writer, domain); err != nil {
				t.Fatalf("encodeDomain() error = %v", err)
			}

			reader := NewBitReader(
				writer.Bytes(),
				writer.BitLen(),
			)

			decoded, err := decodeDomain(reader)
			if err != nil {
				t.Fatalf("decodeDomain() error = %v", err)
			}

			got := decoded.String()

			if got != input && got != input+"." {
				t.Fatalf(
					"expected %q, got %q",
					input,
					got,
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

func TestDomainCodecSize(t *testing.T) {
	domain, err := parseDomain("www.youtube.com")
	if err != nil {
		t.Fatal(err)
	}

	writer := NewBitWriter()

	if err := encodeDomain(writer, domain); err != nil {
		t.Fatal(err)
	}
	// label count                 5 bits

	// www
	// dictionary marker         1 bit
	// token                     3 bits
	// 							= 4

	// youtube
	// raw marker                1 bit
	// length                    6 bits
	// data                      7 × 8
	// 							= 63

	// com
	// dictionary marker         1 bit
	// token                     3 bits
	// 							= 4
	expected := uint64(76)

	if writer.BitLen() != expected {
		t.Fatalf(
			"expected %d bits, got %d",
			expected,
			writer.BitLen(),
		)
	}
}
