package codec

import "fmt"

type RangeDecoder struct {
	data []byte
	pos  int

	low  uint32
	high uint32
	code uint32

	current byte
	nBits   uint8
}

func NewRangeDecoder(data []byte) (*RangeDecoder, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf(
			"range decoder: empty input",
		)
	}

	d := &RangeDecoder{
		data: data,
		high: 0xFFFFFFFF,
	}

	for i := 0; i < 32; i++ {
		d.code = (d.code << 1) | uint32(d.readBit())
	}

	return d, nil
}

func (d *RangeDecoder) readBit() byte {
	if d.nBits == 0 {
		if d.pos >= len(d.data) {
			// Zero-padding after the encoded stream is valid for
			// the arithmetic decoder's final interval.
			return 0
		}

		d.current = d.data[d.pos]
		d.pos++
		d.nBits = 8
	}

	bit := (d.current >> 7) & 1

	d.current <<= 1
	d.nBits--

	return bit
}

func (d *RangeDecoder) Get(total uint64) (uint64, error) {
	if total == 0 {
		return 0, fmt.Errorf(
			"range decoder: zero total frequency",
		)
	}

	width := uint64(d.high-d.low) + 1

	// value is in [0,total).
	value := ((uint64(d.code-d.low)+1)*total - 1) / width

	if value >= total {
		return 0, fmt.Errorf(
			"range decoder: cumulative value %d >= total %d",
			value,
			total,
		)
	}

	return value, nil
}

func (d *RangeDecoder) Decode(
	cumLow,
	cumHigh,
	total uint64,
) error {
	if total == 0 {
		return fmt.Errorf(
			"range decoder: zero total frequency",
		)
	}

	if cumLow >= cumHigh || cumHigh > total {
		return fmt.Errorf(
			"range decoder: invalid interval [%d,%d) total=%d",
			cumLow,
			cumHigh,
			total,
		)
	}

	width := uint64(d.high-d.low) + 1

	newLow := uint64(d.low) +
		(width*cumLow)/total

	newHigh := uint64(d.low) +
		(width*cumHigh)/total -
		1

	d.low = uint32(newLow)
	d.high = uint32(newHigh)

	for {
		switch {
		case d.high < rcHalf:
			// Nothing to do.

		case d.low >= rcHalf:
			d.code -= rcHalf
			d.low -= rcHalf
			d.high -= rcHalf

		case d.low >= rcFirst && d.high < rcThird:
			d.code -= rcFirst
			d.low -= rcFirst
			d.high -= rcFirst

		default:
			return nil
		}

		d.low <<= 1
		d.high = (d.high << 1) | 1
		d.code = (d.code << 1) | uint32(d.readBit())
	}
}

func (d *RangeDecoder) Position() int {
	return d.pos
}
