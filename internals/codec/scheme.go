package codec

import "fmt"

type SchemeCode struct {
	Scheme string
	Code   uint64
	Bits   uint8
}

// Temporary static model.
//
// These codes are deliberately simple for V0.
// Later this table will be generated from URL corpus statistics.
var schemeCodes = []SchemeCode{
	{
		Scheme: "https",
		Code:   0b0,
		Bits:   1,
	},
	{
		Scheme: "http",
		Code:   0b10,
		Bits:   2,
	},
	{
		Scheme: "ftp",
		Code:   0b110,
		Bits:   3,
	},
	{
		Scheme: "ws",
		Code:   0b1110,
		Bits:   4,
	},
	{
		Scheme: "wss",
		Code:   0b11110,
		Bits:   5,
	},
}

var (
	schemeEncodeMap map[string]SchemeCode
	schemeDecodeMap map[string]SchemeCode
)

func init() {
	schemeEncodeMap = make(map[string]SchemeCode)
	schemeDecodeMap = make(map[string]SchemeCode)

	for _, entry := range schemeCodes {
		schemeEncodeMap[entry.Scheme] = entry
	}
}

type schemeDecodeKey struct {
	Code uint64
	Bits uint8
}

var schemeDecodeTable map[schemeDecodeKey]string

func init() {
	schemeEncodeMap = make(map[string]SchemeCode)
	schemeDecodeTable = make(map[schemeDecodeKey]string)

	for _, entry := range schemeCodes {
		schemeEncodeMap[entry.Scheme] = entry

		schemeDecodeTable[schemeDecodeKey{
			Code: entry.Code,
			Bits: entry.Bits,
		}] = entry.Scheme
	}
}

const (
	schemeEscapeCode uint64 = 0b11111
	schemeEscapeBits uint8  = 5
)

func encodeScheme(w *BitWriter, scheme string) error {
	code, ok := schemeEncodeMap[scheme]

	if ok {
		return w.WriteBits(code.Code, code.Bits)
	}

	// Unknown scheme.
	if err := w.WriteBits(schemeEscapeCode, schemeEscapeBits); err != nil {
		return err
	}

	if len(scheme) > 255 {
		return fmt.Errorf("scheme too long: %d bytes", len(scheme))
	}

	// 8-bit length for now.
	if err := w.WriteBits(uint64(len(scheme)), 8); err != nil {
		return err
	}

	for i := 0; i < len(scheme); i++ {
		if err := w.WriteBits(uint64(scheme[i]), 8); err != nil {
			return err
		}
	}

	return nil
}

func decodeScheme(r *BitReader) (string, error) {
	var code uint64

	for bits := uint8(1); bits <= schemeEscapeBits; bits++ {
		bit, err := r.ReadBits(1)
		if err != nil {
			return "", err
		}

		code = (code << 1) | bit

		if scheme, ok := schemeDecodeTable[schemeDecodeKey{
			Code: code,
			Bits: bits,
		}]; ok {
			return scheme, nil
		}

		if bits == schemeEscapeBits && code == schemeEscapeCode {
			break
		}
	}

	// Escape sequence.
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
