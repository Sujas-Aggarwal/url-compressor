package codec

import "fmt"

const (
	rcHalf  uint32 = 0x80000000
	rcFirst uint32 = 0x40000000
	rcThird uint32 = 0xC0000000
)

type RangeEncoder struct {
	low       uint32
	high      uint32
	underflow uint32

	buf     []byte
	current byte
	nBits   uint8
}

func NewRangeEncoder() *RangeEncoder {
	return &RangeEncoder{
		high: 0xFFFFFFFF,
	}
}

func (e *RangeEncoder) Encode(
	cumLow,
	cumHigh,
	total uint64,
) error {
	if total == 0 {
		return fmt.Errorf("range encoder: zero total frequency")
	}

	if cumLow >= cumHigh || cumHigh > total {
		return fmt.Errorf(
			"range encoder: invalid interval [%d,%d) total=%d",
			cumLow,
			cumHigh,
			total,
		)
	}

	width := uint64(e.high-e.low) + 1

	newLow := uint64(e.low) +
		(width*cumLow)/total

	newHigh := uint64(e.low) +
		(width*cumHigh)/total -
		1

	e.low = uint32(newLow)
	e.high = uint32(newHigh)

	for {
		switch {
		case e.high < rcHalf:
			e.outputBit(0)
			e.outputPending(1)

		case e.low >= rcHalf:
			e.outputBit(1)
			e.outputPending(0)

			e.low -= rcHalf
			e.high -= rcHalf

		case e.low >= rcFirst && e.high < rcThird:
			e.underflow++

			e.low -= rcFirst
			e.high -= rcFirst

		default:
			return nil
		}

		e.low <<= 1
		e.high = (e.high << 1) | 1
	}
}

func (e *RangeEncoder) outputBit(bit byte) {
	e.current = (e.current << 1) | bit
	e.nBits++

	if e.nBits == 8 {
		e.buf = append(e.buf, e.current)
		e.current = 0
		e.nBits = 0
	}
}

func (e *RangeEncoder) outputPending(bit byte) {
	for e.underflow > 0 {
		e.outputBit(bit)
		e.underflow--
	}
}

func (e *RangeEncoder) Finish() []byte {
	e.underflow++

	if e.low < rcFirst {
		e.outputBit(0)
		e.outputPending(1)
	} else {
		e.outputBit(1)
		e.outputPending(0)
	}

	if e.nBits != 0 {
		e.current <<= 8 - e.nBits
		e.buf = append(e.buf, e.current)
		e.current = 0
		e.nBits = 0
	}

	result := make([]byte, len(e.buf))
	copy(result, e.buf)

	return result
}

func (e *RangeEncoder) Bytes() []byte {
	return e.Finish()
}
