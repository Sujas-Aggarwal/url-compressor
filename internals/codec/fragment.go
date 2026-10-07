package codec

import "fmt"

type FragmentToken struct {
	Value string
	Code  uint64
	Bits  uint8
}

var fragmentTokens = []FragmentToken{
	{Value: "top", Code: 0b000, Bits: 3},
	{Value: "home", Code: 0b001, Bits: 3},
	{Value: "about", Code: 0b010, Bits: 3},
	{Value: "contact", Code: 0b011, Bits: 3},
	{Value: "pricing", Code: 0b100, Bits: 3},
	{Value: "comments", Code: 0b101, Bits: 3},
	{Value: "reviews", Code: 0b110, Bits: 3},
	{Value: "login", Code: 0b111, Bits: 3},
}

var (
	fragmentTokenEncode map[string]FragmentToken
	fragmentTokenDecode map[uint64]string
)

func init() {
	fragmentTokenEncode = make(map[string]FragmentToken)
	fragmentTokenDecode = make(map[uint64]string)

	for _, token := range fragmentTokens {
		fragmentTokenEncode[token.Value] = token

		key := (uint64(token.Bits) << 56) | token.Code
		fragmentTokenDecode[key] = token.Value
	}
}

func encodeFragment(w *BitWriter, fragment string) error {
	token, ok := fragmentTokenEncode[fragment]

	if ok {
		// Dictionary marker.
		if err := w.WriteBits(0, 1); err != nil {
			return err
		}

		return w.WriteBits(token.Code, token.Bits)
	}

	// Raw marker.
	if err := w.WriteBits(1, 1); err != nil {
		return err
	}

	if len(fragment) > 255 {
		return fmt.Errorf(
			"fragment too long: %d bytes",
			len(fragment),
		)
	}

	if err := w.WriteBits(uint64(len(fragment)), 8); err != nil {
		return err
	}

	for i := 0; i < len(fragment); i++ {
		if err := w.WriteBits(uint64(fragment[i]), 8); err != nil {
			return err
		}
	}

	return nil
}

func decodeFragment(r *BitReader) (string, error) {
	kind, err := r.ReadBits(1)
	if err != nil {
		return "", err
	}

	// Raw fragment.
	if kind == 1 {
		length, err := r.ReadBits(8)
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

	// Dictionary fragment.
	code, err := r.ReadBits(3)
	if err != nil {
		return "", err
	}

	key := (uint64(3) << 56) | code

	fragment, ok := fragmentTokenDecode[key]
	if !ok {
		return "", fmt.Errorf(
			"unknown fragment token: %03b",
			code,
		)
	}

	return fragment, nil
}
