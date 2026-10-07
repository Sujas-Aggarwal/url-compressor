package codec

import "fmt"

type BitWriter struct {
	data    []byte
	current byte
	nBits   uint8
	bitLen  uint64
}

func NewBitWriter() *BitWriter {
	return &BitWriter{}
}

// WriteBits writes the lowest n bits of value into the stream.
//
// Bits are written most-significant-bit first.
func (w *BitWriter) WriteBits(value uint64, n uint8) error {
	if n > 64 {
		return fmt.Errorf("cannot write more than 64 bits")
	}

	if n == 0 {
		return nil
	}

	for i := int(n) - 1; i >= 0; i-- {
		bit := (value >> i) & 1

		w.current <<= 1
		w.current |= byte(bit)
		w.nBits++
		w.bitLen++

		if w.nBits == 8 {
			w.data = append(w.data, w.current)
			w.current = 0
			w.nBits = 0
		}
	}

	return nil
}

// Bytes returns the encoded bitstream.
//
// If the number of bits is not a multiple of 8, the remaining
// bits are padded with zeroes in the final byte.
func (w *BitWriter) Bytes() []byte {
	result := make([]byte, len(w.data))

	copy(result, w.data)

	if w.nBits > 0 {
		result = append(result, w.current<<(8-w.nBits))
	}

	return result
}

func (w *BitWriter) BitLen() uint64 {
	return w.bitLen
}
