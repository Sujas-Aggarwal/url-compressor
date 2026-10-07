package codec

import "fmt"

type RangeDecoder struct {
	data   []byte
	pos    int
	code   uint64
	range_ uint64
}

func NewRangeDecoder(data []byte) (*RangeDecoder, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf(
			"range decoder: need at least 8 bytes, got %d",
			len(data),
		)
	}

	d := &RangeDecoder{
		data:   data,
		range_: rangeMax,
	}

	for i := 0; i < 8; i++ {
		d.code = (d.code << 8) | uint64(data[i])
	}

	d.pos = 8

	return d, nil
}

// Get returns the cumulative-frequency position of the next symbol.
//
// IMPORTANT:
// This does not modify decoder state.
// Decode() must be called afterward with the selected interval.
func (d *RangeDecoder) Get(total uint64) (uint64, error) {
	if total == 0 {
		return 0, fmt.Errorf("range decoder: zero total frequency")
	}

	scaledRange := d.range_ / total

	if scaledRange == 0 {
		return 0, fmt.Errorf("range decoder: zero range")
	}

	return d.code / scaledRange, nil
}

func (d *RangeDecoder) Decode(
	cumLow,
	cumHigh,
	total uint64,
) error {
	if total == 0 {
		return fmt.Errorf("range decoder: zero total frequency")
	}

	if cumLow >= cumHigh || cumHigh > total {
		return fmt.Errorf(
			"range decoder: invalid frequencies low=%d high=%d total=%d",
			cumLow,
			cumHigh,
			total,
		)
	}

	scaledRange := d.range_ / total

	if scaledRange == 0 {
		return fmt.Errorf("range decoder: zero range")
	}

	d.code -= scaledRange * cumLow
	d.range_ = scaledRange * (cumHigh - cumLow)

	for d.range_ < rangeBottom {
		if d.pos >= len(d.data) {
			return fmt.Errorf(
				"range decoder: unexpected end of input",
			)
		}

		d.code = (d.code << 8) | uint64(d.data[d.pos])
		d.pos++

		d.range_ <<= 8
	}

	return nil
}

func (d *RangeDecoder) Position() int {
	return d.pos
}
