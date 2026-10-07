package codec

import (
	"encoding/binary"
	"fmt"
)

type StaticPPMEncoder struct {
	model *StaticPPM
}

type StaticPPMDecoder struct {
	model *StaticPPM
}

func NewStaticPPMEncoder(model *StaticPPM) *StaticPPMEncoder {
	return &StaticPPMEncoder{
		model: model,
	}
}

func NewStaticPPMDecoder(model *StaticPPM) *StaticPPMDecoder {
	return &StaticPPMDecoder{
		model: model,
	}
}

// Encode compresses data using the corpus-trained static PPM model.
//
// Format:
//
//	4 bytes: original uncompressed length
//	remaining bytes: range-coded PPM stream
func (e *StaticPPMEncoder) Encode(data []byte) ([]byte, error) {
	if e == nil || e.model == nil {
		return nil, fmt.Errorf("nil PPM encoder/model")
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("cannot encode empty input")
	}

	if uint64(len(data)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("input too large: %d bytes", len(data))
	}

	rangeEncoder := NewRangeEncoder()

	history := make([]byte, 0, int(e.model.MaxOrder))

	for _, symbol := range data {
		if err := encodeStaticPPMSymbol(
			rangeEncoder,
			e.model,
			history,
			symbol,
		); err != nil {
			return nil, fmt.Errorf("encode symbol %q: %w", symbol, err)
		}

		history = appendHistory(
			history,
			symbol,
			int(e.model.MaxOrder),
		)
	}

	rangeBytes := rangeEncoder.Finish()

	result := make([]byte, 4, 4+len(rangeBytes))
	binary.BigEndian.PutUint32(result, uint32(len(data)))
	result = append(result, rangeBytes...)

	return result, nil
}

// Decode decompresses data produced by StaticPPMEncoder.
func (d *StaticPPMDecoder) Decode(data []byte) ([]byte, error) {
	if d == nil || d.model == nil {
		return nil, fmt.Errorf("nil PPM decoder/model")
	}

	if len(data) < 4 {
		return nil, fmt.Errorf("compressed data too short")
	}

	originalLength := binary.BigEndian.Uint32(data[:4])

	if originalLength == 0 {
		return nil, fmt.Errorf("invalid original length: 0")
	}

	rangeData := data[4:]

	if len(rangeData) == 0 {
		return nil, fmt.Errorf("missing range-coded data")
	}

	rangeDecoder, err := NewRangeDecoder(rangeData)
	if err != nil {
		return nil, fmt.Errorf("initialize range decoder: %w", err)
	}

	output := make([]byte, 0, int(originalLength))
	history := make([]byte, 0, int(d.model.MaxOrder))

	for i := uint32(0); i < originalLength; i++ {
		symbol, err := decodeStaticPPMSymbol(
			rangeDecoder,
			d.model,
			history,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"decode symbol %d: %w",
				i,
				err,
			)
		}

		output = append(output, symbol)

		history = appendHistory(
			history,
			symbol,
			int(d.model.MaxOrder),
		)
	}

	return output, nil
}

func encodeStaticPPMSymbol(
	encoder *RangeEncoder,
	model *StaticPPM,
	history []byte,
	symbol byte,
) error {
	maxOrder := int(model.MaxOrder)

	if len(history) < maxOrder {
		maxOrder = len(history)
	}

	for order := maxOrder; order >= 0; order-- {
		context := model.Lookup(history, order)

		if context == nil {
			continue
		}

		low, high, ok := context.Find(symbol)

		// Every context has an implicit ESC with count 1.
		//
		// Symbols: [0, context.Total)
		// ESC:     [context.Total, context.Total+1)
		// Total:   context.Total+1
		total := uint64(context.Total) + 1

		if ok {
			return encoder.Encode(
				uint64(low),
				uint64(high),
				total,
			)
		}

		// Symbol is not present in this context.
		// Emit ESC and back off.
		escapeLow := uint64(context.Total)
		escapeHigh := escapeLow + 1

		if err := encoder.Encode(
			escapeLow,
			escapeHigh,
			total,
		); err != nil {
			return err
		}

		// At order 0, ESC means the byte was never observed
		// during training. Encode the raw byte using a uniform
		// 256-symbol alphabet.
		if order == 0 {
			return encodeRawByte(encoder, symbol)
		}
	}

	// Defensive fallback if no context exists.
	return encodeRawByte(encoder, symbol)
}

func decodeStaticPPMSymbol(
	decoder *RangeDecoder,
	model *StaticPPM,
	history []byte,
) (byte, error) {
	maxOrder := int(model.MaxOrder)

	if len(history) < maxOrder {
		maxOrder = len(history)
	}

	for order := maxOrder; order >= 0; order-- {
		context := model.Lookup(history, order)

		if context == nil {
			continue
		}

		// Every context has an implicit ESC.
		total := uint64(context.Total) + 1

		value, err := decoder.Get(total)
		if err != nil {
			return 0, err
		}

		// Normal symbol.
		if value < uint64(context.Total) {
			symbol, low, high, ok := context.SymbolAt(uint32(value))

			if !ok {
				return 0, fmt.Errorf(
					"invalid symbol interval: value=%d total=%d",
					value,
					context.Total,
				)
			}

			if err := decoder.Decode(
				uint64(low),
				uint64(high),
				total,
			); err != nil {
				return 0, err
			}

			return symbol, nil
		}

		// ESC.
		escapeLow := uint64(context.Total)
		escapeHigh := escapeLow + 1

		if err := decoder.Decode(
			escapeLow,
			escapeHigh,
			total,
		); err != nil {
			return 0, err
		}

		// At order 0, ESC means the symbol was unseen during
		// training, so decode a raw byte.
		if order == 0 {
			return decodeRawByte(decoder)
		}
	}

	return decodeRawByte(decoder)
}

func encodeRawByte(
	encoder *RangeEncoder,
	symbol byte,
) error {
	return encoder.Encode(
		uint64(symbol),
		uint64(symbol)+1,
		256,
	)
}

func decodeRawByte(
	decoder *RangeDecoder,
) (byte, error) {
	value, err := decoder.Get(256)
	if err != nil {
		return 0, err
	}

	symbol := byte(value)

	if err := decoder.Decode(
		uint64(symbol),
		uint64(symbol)+1,
		256,
	); err != nil {
		return 0, err
	}

	return symbol, nil
}

func appendHistory(
	history []byte,
	symbol byte,
	maxOrder int,
) []byte {
	if maxOrder == 0 {
		return history[:0]
	}

	if len(history) < maxOrder {
		return append(history, symbol)
	}

	copy(history, history[1:])
	history[len(history)-1] = symbol

	return history
}
