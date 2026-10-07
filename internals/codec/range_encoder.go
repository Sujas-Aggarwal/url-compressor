package codec

import "fmt"

const (
	rangeTop    uint64 = 1 << 56
	rangeBottom uint64 = 1 << 48
	rangeMax    uint64 = ^uint64(0)
)

type RangeEncoder struct {
	buf    []byte
	low    uint64
	range_ uint64
}

func NewRangeEncoder() *RangeEncoder {
	return &RangeEncoder{
		low:    0,
		range_: rangeMax,
	}
}

func (e *RangeEncoder) Encode(cumLow, cumHigh, total uint64) error {
	if total == 0 {
		return fmt.Errorf("range encoder: zero total frequency")
	}

	if cumLow >= cumHigh || cumHigh > total {
		return fmt.Errorf(
			"range encoder: invalid frequencies low=%d high=%d total=%d",
			cumLow,
			cumHigh,
			total,
		)
	}

	e.range_ /= total
	e.low += e.range_ * cumLow
	e.range_ *= cumHigh - cumLow

	for e.range_ < rangeBottom {
		e.buf = append(e.buf, byte(e.low>>56))
		e.low <<= 8
		e.range_ <<= 8
	}

	return nil
}

func (e *RangeEncoder) Finish() []byte {
	// Emit enough high bits of low to uniquely identify the
	// final coding interval.
	for i := 0; i < 8; i++ {
		e.buf = append(e.buf, byte(e.low>>56))
		e.low <<= 8
	}

	result := make([]byte, len(e.buf))
	copy(result, e.buf)

	return result
}

func (e *RangeEncoder) Bytes() []byte {
	return e.Finish()
}
