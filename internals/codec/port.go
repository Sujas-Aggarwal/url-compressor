package codec

import "fmt"

type PortToken struct {
	Port uint16
	Code uint64
	Bits uint8
}

var portTokens = []PortToken{
	{Port: 80, Code: 0b000, Bits: 3},
	{Port: 443, Code: 0b001, Bits: 3},
	{Port: 8080, Code: 0b010, Bits: 3},
	{Port: 8443, Code: 0b011, Bits: 3},
	{Port: 3000, Code: 0b100, Bits: 3},
	{Port: 5000, Code: 0b101, Bits: 3},
	{Port: 8000, Code: 0b110, Bits: 3},
	{Port: 9000, Code: 0b111, Bits: 3},
}

var (
	portEncodeMap map[uint16]PortToken
	portDecodeMap map[uint64]uint16
)

func init() {
	portEncodeMap = make(map[uint16]PortToken)
	portDecodeMap = make(map[uint64]uint16)

	for _, token := range portTokens {
		portEncodeMap[token.Port] = token

		key := (uint64(token.Bits) << 56) | token.Code
		portDecodeMap[key] = token.Port
	}
}

func encodePort(w *BitWriter, port uint16) error {
	token, ok := portEncodeMap[port]

	if ok {
		if err := w.WriteBits(0, 1); err != nil {
			return err
		}

		return w.WriteBits(token.Code, token.Bits)
	}

	// Raw port.
	if err := w.WriteBits(1, 1); err != nil {
		return err
	}

	return w.WriteBits(uint64(port), 16)
}

func decodePort(r *BitReader) (uint16, error) {
	kind, err := r.ReadBits(1)
	if err != nil {
		return 0, err
	}

	// Raw port.
	if kind == 1 {
		value, err := r.ReadBits(16)
		if err != nil {
			return 0, err
		}

		return uint16(value), nil
	}

	// Dictionary token.
	code, err := r.ReadBits(3)
	if err != nil {
		return 0, err
	}

	key := (uint64(3) << 56) | code

	port, ok := portDecodeMap[key]
	if !ok {
		return 0, fmt.Errorf(
			"unknown port token: %03b",
			code,
		)
	}

	return port, nil
}
