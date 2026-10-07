package codec

import (
	"fmt"
	"strings"
	"testing"
)

type Domain struct {
	Labels []string
}

func parseDomain(host string) (*Domain, error) {
	if host == "" {
		return &Domain{}, nil
	}

	host = strings.TrimSuffix(host, ".")

	if host == "" {
		return &Domain{}, nil
	}

	labels := strings.Split(host, ".")

	for _, label := range labels {
		if label == "" {
			return nil, fmt.Errorf("invalid domain: empty label")
		}

		if len(label) > 63 {
			return nil, fmt.Errorf(
				"invalid domain label %q: length %d exceeds 63",
				label,
				len(label),
			)
		}
	}

	return &Domain{
		Labels: labels,
	}, nil
}

func (d *Domain) String() string {
	return strings.Join(d.Labels, ".")
}

func TestDomainParse(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "www.youtube.com",
			expected: []string{"www", "youtube", "com"},
		},
		{
			input:    "api.v2.example.co.uk",
			expected: []string{"api", "v2", "example", "co", "uk"},
		},
		{
			input:    "localhost",
			expected: []string{"localhost"},
		},
	}

	for _, tc := range tests {
		domain, err := parseDomain(tc.input)
		if err != nil {
			t.Fatal(err)
		}

		if len(domain.Labels) != len(tc.expected) {
			t.Fatalf(
				"expected %d labels, got %d",
				len(tc.expected),
				len(domain.Labels),
			)
		}

		for i := range tc.expected {
			if domain.Labels[i] != tc.expected[i] {
				t.Fatalf(
					"expected label %q, got %q",
					tc.expected[i],
					domain.Labels[i],
				)
			}
		}

		if domain.String() != tc.input {
			t.Fatalf(
				"expected %q, got %q",
				tc.input,
				domain.String(),
			)
		}
	}
}
