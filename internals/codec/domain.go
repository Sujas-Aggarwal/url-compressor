package codec

import (
	"fmt"
	"strings"
	"testing"
)

const (
	domainLabelCountBits  = 5
	domainLabelLengthBits = 6
)

type Domain struct {
	Labels []string
}
type DomainToken struct {
	Value string
	Code  uint64
	Bits  uint8
}

var domainTokens = []DomainToken{
	{Value: "com", Code: 0b000, Bits: 3},
	{Value: "org", Code: 0b001, Bits: 3},
	{Value: "net", Code: 0b010, Bits: 3},
	{Value: "io", Code: 0b011, Bits: 3},
	{Value: "in", Code: 0b100, Bits: 3},
	{Value: "co", Code: 0b101, Bits: 3},
	{Value: "uk", Code: 0b110, Bits: 3},
	{Value: "www", Code: 0b111, Bits: 3},
}

var (
	domainTokenEncode map[string]DomainToken
	domainTokenDecode map[uint64]string
)

func init() {
	domainTokenEncode = make(map[string]DomainToken)
	domainTokenDecode = make(map[uint64]string)

	for _, token := range domainTokens {
		domainTokenEncode[token.Value] = token

		key := (uint64(token.Bits) << 56) | token.Code
		domainTokenDecode[key] = token.Value
	}
}

const (
	domainEscapeCode uint64 = 0b11111
	domainEscapeBits uint8  = 5
)

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

func encodeDomain(w *BitWriter, domain *Domain) error {
	if len(domain.Labels) > 31 {
		return fmt.Errorf(
			"too many domain labels: %d",
			len(domain.Labels),
		)
	}

	if err := w.WriteBits(
		uint64(len(domain.Labels)),
		domainLabelCountBits,
	); err != nil {
		return err
	}

	for _, label := range domain.Labels {
		if err := encodeDomainLabel(w, label); err != nil {
			return err
		}
	}

	return nil
}

func decodeDomain(r *BitReader) (*Domain, error) {
	count, err := r.ReadBits(domainLabelCountBits)
	if err != nil {
		return nil, err
	}

	domain := &Domain{
		Labels: make([]string, 0, count),
	}

	for i := uint64(0); i < count; i++ {
		label, err := decodeDomainLabel(r)
		if err != nil {
			return nil, err
		}

		domain.Labels = append(domain.Labels, label)
	}

	return domain, nil
}

func encodeDomainLabel(w *BitWriter, label string) error {
	if token, ok := domainTokenEncode[label]; ok {
		// 0 = dictionary token
		if err := w.WriteBits(0, 1); err != nil {
			return err
		}

		if err := w.WriteBits(token.Code, token.Bits); err != nil {
			return err
		}

		return nil
	}

	// 1 = raw label
	if err := w.WriteBits(1, 1); err != nil {
		return err
	}

	if len(label) > 63 {
		return fmt.Errorf("domain label too long: %q", label)
	}

	if err := w.WriteBits(uint64(len(label)), 6); err != nil {
		return err
	}

	for i := 0; i < len(label); i++ {
		if err := w.WriteBits(uint64(label[i]), 8); err != nil {
			return err
		}
	}

	return nil
}

func decodeDomainLabel(r *BitReader) (string, error) {
	kind, err := r.ReadBits(1)
	if err != nil {
		return "", err
	}

	if kind == 1 {
		length, err := r.ReadBits(6)
		if err != nil {
			return "", err
		}

		data := make([]byte, length)

		for i := range data {
			value, err := r.ReadBits(8)
			if err != nil {
				return "", err
			}

			data[i] = byte(value)
		}

		return string(data), nil
	}

	code, err := r.ReadBits(3)
	if err != nil {
		return "", err
	}

	key := (uint64(3) << 56) | code

	label, ok := domainTokenDecode[key]
	if !ok {
		return "", fmt.Errorf(
			"unknown domain token: %03b",
			code,
		)
	}

	return label, nil
}
