package codec

import (
	"encoding/binary"
	"fmt"
)

const (
	PPMMaxOrder = 4
	PPMEscape   = 256

	ppmMaxCount = 65535
)

// ppmContext stores symbol frequencies for one context.
type ppmContext struct {
	Counts map[byte]uint32
	Total  uint32
}

// PPMModel is an adaptive PPM model.
//
// The model itself is the trained starting state.
// Encode and Decode clone the model so both sides evolve
// independently but identically.
type PPMModel struct {
	orders [PPMMaxOrder + 1]map[string]*ppmContext
	global *ppmContext
}

func NewPPMModel() *PPMModel {
	m := &PPMModel{
		global: newPPMContext(),
	}

	for i := 0; i <= PPMMaxOrder; i++ {
		m.orders[i] = make(map[string]*ppmContext)
	}

	return m
}

func newPPMContext() *ppmContext {
	return &ppmContext{
		Counts: make(map[byte]uint32),
	}
}

func (m *PPMModel) Reset() {
	for i := 0; i <= PPMMaxOrder; i++ {
		m.orders[i] = make(map[string]*ppmContext)
	}

	m.global = newPPMContext()
}

// Update trains the model on data.
func (m *PPMModel) Update(data []byte) {
	history := make([]byte, 0, PPMMaxOrder)

	for _, symbol := range data {
		m.updateSymbol(history, symbol)

		history = append(history, symbol)

		if len(history) > PPMMaxOrder {
			history = history[len(history)-PPMMaxOrder:]
		}
	}
}

func (m *PPMModel) updateSymbol(
	history []byte,
	symbol byte,
) {
	m.updateContext(m.global, symbol)

	maxOrder := len(history)

	if maxOrder > PPMMaxOrder {
		maxOrder = PPMMaxOrder
	}

	for order := 1; order <= maxOrder; order++ {
		start := len(history) - order
		key := string(history[start:])

		ctx := m.orders[order][key]

		if ctx == nil {
			ctx = newPPMContext()
			m.orders[order][key] = ctx
		}

		m.updateContext(ctx, symbol)
	}
}

func (m *PPMModel) updateContext(
	ctx *ppmContext,
	symbol byte,
) {
	if ctx.Counts[symbol] >= ppmMaxCount {
		return
	}

	ctx.Counts[symbol]++
	ctx.Total++

	if ctx.Total >= ppmMaxCount {
		m.rescaleContext(ctx)
	}
}

func (m *PPMModel) rescaleContext(ctx *ppmContext) {
	var total uint32

	for symbol, count := range ctx.Counts {
		count = (count + 1) / 2

		if count == 0 {
			delete(ctx.Counts, symbol)
			continue
		}

		ctx.Counts[symbol] = count
		total += count
	}

	ctx.Total = total
}

func (m *PPMModel) context(
	history []byte,
	order int,
) *ppmContext {
	if order == 0 {
		return m.global
	}

	if len(history) < order {
		return nil
	}

	start := len(history) - order
	key := string(history[start:])

	return m.orders[order][key]
}

// Encode compresses data using the current PPM model.
//
// Format:
//
//	[4 bytes: original length]
//	[range-coded payload]
//
// We deliberately use an explicit length rather than an EOF symbol.
// This gives the decoder an unambiguous termination condition.
func (m *PPMModel) Encode(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot encode empty data")
	}

	if uint64(len(data)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("input too large")
	}

	model := m.Clone()

	encoder := NewRangeEncoder()

	history := make([]byte, 0, PPMMaxOrder)

	for _, symbol := range data {
		if err := model.encodeSymbol(
			encoder,
			history,
			symbol,
		); err != nil {
			return nil, err
		}

		model.updateSymbol(history, symbol)

		history = append(history, symbol)

		if len(history) > PPMMaxOrder {
			history = history[len(history)-PPMMaxOrder:]
		}
	}

	payload := encoder.Finish()

	result := make([]byte, 4+len(payload))

	binary.BigEndian.PutUint32(
		result[:4],
		uint32(len(data)),
	)

	copy(result[4:], payload)

	return result, nil
}

func (m *PPMModel) encodeSymbol(
	encoder *RangeEncoder,
	history []byte,
	symbol byte,
) error {
	maxOrder := len(history)

	if maxOrder > PPMMaxOrder {
		maxOrder = PPMMaxOrder
	}

	for order := maxOrder; order >= 0; order-- {
		ctx := m.context(history, order)

		if ctx == nil || ctx.Total == 0 {
			continue
		}

		count, exists := ctx.Counts[symbol]

		if exists {
			cumulative := m.cumulative(
				ctx,
				symbol,
			)

			total := uint64(ctx.Total + 1)

			if err := encoder.Encode(
				uint64(cumulative),
				uint64(cumulative+count),
				total,
			); err != nil {
				return err
			}

			return nil
		}

		// ESCAPE.
		//
		// Layout:
		//
		// [symbols][ESC]
		//
		if err := encoder.Encode(
			uint64(ctx.Total),
			uint64(ctx.Total+1),
			uint64(ctx.Total+1),
		); err != nil {
			return err
		}
	}

	return fmt.Errorf(
		"symbol %d not present in PPM model",
		symbol,
	)
}

func (m *PPMModel) cumulative(
	ctx *ppmContext,
	symbol byte,
) uint32 {
	var cumulative uint32

	for s := 0; s < int(symbol); s++ {
		cumulative += ctx.Counts[byte(s)]
	}

	return cumulative
}

// Decode decompresses data encoded by Encode.
//
// The first four bytes give the exact number of symbols to decode.
// Therefore this function cannot accidentally run past the encoded
// stream.
func (m *PPMModel) Decode(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf(
			"invalid PPM stream: missing length",
		)
	}

	length := binary.BigEndian.Uint32(data[:4])

	if length == 0 {
		return nil, fmt.Errorf(
			"invalid PPM stream: zero length",
		)
	}

	if len(data) == 4 {
		return nil, fmt.Errorf(
			"invalid PPM stream: missing range payload",
		)
	}

	model := m.Clone()

	decoder, err := NewRangeDecoder(data[4:])
	if err != nil {
		return nil, err
	}

	result := make([]byte, 0, int(length))
	history := make([]byte, 0, PPMMaxOrder)

	for i := uint32(0); i < length; i++ {
		symbol, err := model.decodeSymbol(
			decoder,
			history,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"decode symbol %d/%d: %w",
				i,
				length,
				err,
			)
		}

		b := byte(symbol)

		result = append(result, b)

		model.updateSymbol(history, b)

		history = append(history, b)

		if len(history) > PPMMaxOrder {
			history = history[len(history)-PPMMaxOrder:]
		}
	}

	return result, nil
}

func (m *PPMModel) decodeSymbol(
	decoder *RangeDecoder,
	history []byte,
) (byte, error) {
	maxOrder := len(history)

	if maxOrder > PPMMaxOrder {
		maxOrder = PPMMaxOrder
	}

	for order := maxOrder; order >= 0; order-- {
		ctx := m.context(history, order)

		if ctx == nil || ctx.Total == 0 {
			continue
		}

		total := uint64(ctx.Total + 1)

		value, err := decoder.Get(total)
		if err != nil {
			return 0, err
		}

		symbol, low, high, err := decodeFromContext(
			ctx,
			value,
		)
		if err != nil {
			return 0, err
		}

		if err := decoder.Decode(
			low,
			high,
			total,
		); err != nil {
			return 0, err
		}

		if symbol == PPMEscape {
			continue
		}

		return byte(symbol), nil
	}

	return 0, fmt.Errorf(
		"PPM decoder exhausted all contexts",
	)
}

func decodeFromContext(
	ctx *ppmContext,
	value uint64,
) (
	symbol int,
	cumLow uint64,
	cumHigh uint64,
	err error,
) {
	total := uint64(ctx.Total + 1)

	if value >= total {
		return 0, 0, 0, fmt.Errorf(
			"invalid PPM cumulative value %d (total %d)",
			value,
			total,
		)
	}

	var cumulative uint32

	for s := 0; s < 256; s++ {
		count := ctx.Counts[byte(s)]

		if count == 0 {
			continue
		}

		next := cumulative + count

		if value < uint64(next) {
			return s,
				uint64(cumulative),
				uint64(next),
				nil
		}

		cumulative = next
	}

	// ESCAPE occupies the final interval.
	if value == uint64(ctx.Total) {
		return PPMEscape,
			uint64(ctx.Total),
			uint64(ctx.Total + 1),
			nil
	}

	return 0, 0, 0, fmt.Errorf(
		"invalid PPM cumulative value %d",
		value,
	)
}

func (m *PPMModel) ContextCount(order int) int {
	if order < 0 || order > PPMMaxOrder {
		return 0
	}

	if order == 0 {
		if m.global == nil {
			return 0
		}

		return 1
	}

	return len(m.orders[order])
}

func (m *PPMModel) Clone() *PPMModel {
	clone := NewPPMModel()

	for order := 0; order <= PPMMaxOrder; order++ {
		for key, ctx := range m.orders[order] {
			copied := newPPMContext()
			copied.Total = ctx.Total

			for symbol, count := range ctx.Counts {
				copied.Counts[symbol] = count
			}

			clone.orders[order][key] = copied
		}
	}

	clone.global.Total = m.global.Total

	for symbol, count := range m.global.Counts {
		clone.global.Counts[symbol] = count
	}

	return clone
}
