package codec

import "fmt"

type BitReader struct {
	data   []byte
	bitPos uint64
	bitLen uint64
}

func NewBitReader(data []byte, bitLen uint64) *BitReader {
	return &BitReader{
		data:   data,
		bitLen: bitLen,
	}
}

func (r *BitReader) ReadBits(n uint8) (uint64, error) {
	if n > 64 {
		return 0, fmt.Errorf("cannot read more than 64 bits")
	}

	if uint64(n)+r.bitPos > r.bitLen {
		return 0, fmt.Errorf(
			"not enough bits: requested %d, remaining %d",
			n,
			r.Remaining(),
		)
	}

	var value uint64

	for i := uint8(0); i < n; i++ {
		byteIndex := r.bitPos / 8
		bitIndex := 7 - (r.bitPos % 8)

		bit := (r.data[byteIndex] >> bitIndex) & 1

		value <<= 1
		value |= uint64(bit)

		r.bitPos++
	}

	return value, nil
}

func (r *BitReader) Remaining() uint64 {
	return r.bitLen - r.bitPos
}
