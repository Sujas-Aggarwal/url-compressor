package codec

import (
	"fmt"
	"strings"
)

type PathToken struct {
	Value string
	Code  uint64
	Bits  uint8
}

var pathTokens = []PathToken{
	{Value: "api", Code: 0b000, Bits: 3},
	{Value: "v1", Code: 0b001, Bits: 3},
	{Value: "v2", Code: 0b010, Bits: 3},
	{Value: "users", Code: 0b011, Bits: 3},
	{Value: "user", Code: 0b100, Bits: 3},
	{Value: "products", Code: 0b101, Bits: 3},
	{Value: "product", Code: 0b110, Bits: 3},
	{Value: "search", Code: 0b111, Bits: 3},
}
var (
	pathTokenEncode map[string]PathToken
	pathTokenDecode map[uint64]string
)

func init() {
	pathTokenEncode = make(map[string]PathToken)
	pathTokenDecode = make(map[uint64]string)

	for _, token := range pathTokens {
		pathTokenEncode[token.Value] = token

		key := (uint64(token.Bits) << 56) | token.Code
		pathTokenDecode[key] = token.Value
	}
}

type Path struct {
	Segments []string
	Leading  bool
	Trailing bool
}

func parsePath(raw string) (*Path, error) {
	if raw == "" {
		return &Path{}, nil
	}

	path := &Path{
		Leading:  strings.HasPrefix(raw, "/"),
		Trailing: strings.HasSuffix(raw, "/"),
	}

	trimmed := raw

	if path.Leading {
		trimmed = trimmed[1:]
	}

	if path.Trailing && trimmed != "" {
		trimmed = trimmed[:len(trimmed)-1]
	}

	if trimmed == "" {
		return path, nil
	}

	path.Segments = strings.Split(trimmed, "/")

	return path, nil
}

func (p *Path) String() string {
	if len(p.Segments) == 0 {
		if p.Leading {
			return "/"
		}

		return ""
	}

	var builder strings.Builder

	if p.Leading {
		builder.WriteByte('/')
	}

	builder.WriteString(strings.Join(p.Segments, "/"))

	if p.Trailing {
		builder.WriteByte('/')
	}

	return builder.String()
}

func encodePathSegment(w *BitWriter, segment string) error {
	if token, ok := pathTokenEncode[segment]; ok {
		if err := w.WriteBits(0, 1); err != nil {
			return err
		}

		return w.WriteBits(token.Code, token.Bits)
	}

	if err := w.WriteBits(1, 1); err != nil {
		return err
	}

	if len(segment) > 63 {
		return fmt.Errorf(
			"path segment too long: %q",
			segment,
		)
	}

	if err := w.WriteBits(uint64(len(segment)), 6); err != nil {
		return err
	}

	for i := 0; i < len(segment); i++ {
		if err := w.WriteBits(uint64(segment[i]), 8); err != nil {
			return err
		}
	}

	return nil
}

func decodePathSegment(r *BitReader) (string, error) {
	kind, err := r.ReadBits(1)
	if err != nil {
		return "", err
	}

	// Raw segment.
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

	// Dictionary token.
	code, err := r.ReadBits(3)
	if err != nil {
		return "", err
	}

	key := (uint64(3) << 56) | code

	segment, ok := pathTokenDecode[key]
	if !ok {
		return "", fmt.Errorf(
			"unknown path token: %03b",
			code,
		)
	}

	return segment, nil
}

func encodePath(w *BitWriter, path *Path) error {
	if len(path.Segments) > 31 {
		return fmt.Errorf(
			"too many path segments: %d",
			len(path.Segments),
		)
	}

	// Leading slash.
	if path.Leading {
		if err := w.WriteBits(1, 1); err != nil {
			return err
		}
	} else {
		if err := w.WriteBits(0, 1); err != nil {
			return err
		}
	}

	// Trailing slash.
	if path.Trailing {
		if err := w.WriteBits(1, 1); err != nil {
			return err
		}
	} else {
		if err := w.WriteBits(0, 1); err != nil {
			return err
		}
	}

	// Number of segments.
	if err := w.WriteBits(
		uint64(len(path.Segments)),
		5,
	); err != nil {
		return err
	}

	for _, segment := range path.Segments {
		if err := encodePathSegment(w, segment); err != nil {
			return err
		}
	}

	return nil
}

func decodePath(r *BitReader) (*Path, error) {
	leading, err := r.ReadBits(1)
	if err != nil {
		return nil, err
	}

	trailing, err := r.ReadBits(1)
	if err != nil {
		return nil, err
	}

	count, err := r.ReadBits(5)
	if err != nil {
		return nil, err
	}

	path := &Path{
		Leading:  leading == 1,
		Trailing: trailing == 1,
		Segments: make([]string, 0, count),
	}

	for i := uint64(0); i < count; i++ {
		segment, err := decodePathSegment(r)
		if err != nil {
			return nil, err
		}

		path.Segments = append(
			path.Segments,
			segment,
		)
	}

	return path, nil
}
