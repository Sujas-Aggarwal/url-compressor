package codec

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	staticPPMMagic   = "PPM3"
	staticPPMVersion = 1
)

type StaticPPMSymbol struct {
	Symbol byte
	Count  uint32
}

type StaticPPMContext struct {
	Key     uint32
	Order   uint8
	Total   uint32
	Symbols []StaticPPMSymbol
}

type StaticPPM struct {
	MaxOrder uint8

	contexts map[uint64]*StaticPPMContext
}

func NewStaticPPM() *StaticPPM {
	return &StaticPPM{
		contexts: make(map[uint64]*StaticPPMContext),
	}
}

func staticPPMContextKey(order uint8, key uint32) uint64 {
	return (uint64(order) << 32) | uint64(key)
}

func LoadStaticPPM(path string) (*StaticPPM, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open PPM model: %w", err)
	}
	defer file.Close()

	model := NewStaticPPM()

	var magic [4]byte

	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return nil, fmt.Errorf("read PPM magic: %w", err)
	}

	if string(magic[:]) != staticPPMMagic {
		return nil, fmt.Errorf(
			"invalid PPM magic %q",
			string(magic[:]),
		)
	}

	var version uint8

	if err := binary.Read(
		file,
		binary.BigEndian,
		&version,
	); err != nil {
		return nil, fmt.Errorf("read PPM version: %w", err)
	}

	if version != staticPPMVersion {
		return nil, fmt.Errorf(
			"unsupported PPM version %d",
			version,
		)
	}

	var maxOrder uint8

	if err := binary.Read(
		file,
		binary.BigEndian,
		&maxOrder,
	); err != nil {
		return nil, fmt.Errorf(
			"read PPM max order: %w",
			err,
		)
	}

	if maxOrder > 3 {
		return nil, fmt.Errorf(
			"unsupported PPM max order %d",
			maxOrder,
		)
	}

	model.MaxOrder = maxOrder

	// Training percentage.
	var trainPercent uint8

	if err := binary.Read(
		file,
		binary.BigEndian,
		&trainPercent,
	); err != nil {
		return nil, fmt.Errorf(
			"read PPM train percentage: %w",
			err,
		)
	}

	// Reserved byte.
	var reserved uint8

	if err := binary.Read(
		file,
		binary.BigEndian,
		&reserved,
	); err != nil {
		return nil, fmt.Errorf(
			"read PPM reserved byte: %w",
			err,
		)
	}

	_ = trainPercent
	_ = reserved

	var totalURLs uint64
	var trainURLs uint64
	var contextCount uint64

	if err := binary.Read(
		file,
		binary.BigEndian,
		&totalURLs,
	); err != nil {
		return nil, fmt.Errorf(
			"read total URL count: %w",
			err,
		)
	}

	if err := binary.Read(
		file,
		binary.BigEndian,
		&trainURLs,
	); err != nil {
		return nil, fmt.Errorf(
			"read training URL count: %w",
			err,
		)
	}

	if err := binary.Read(
		file,
		binary.BigEndian,
		&contextCount,
	); err != nil {
		return nil, fmt.Errorf(
			"read context count: %w",
			err,
		)
	}

	if contextCount == 0 {
		return nil, fmt.Errorf(
			"PPM model contains no contexts",
		)
	}

	if contextCount > 10_000_000 {
		return nil, fmt.Errorf(
			"PPM context count is unreasonable: %d",
			contextCount,
		)
	}

	_ = totalURLs
	_ = trainURLs

	model.contexts = make(
		map[uint64]*StaticPPMContext,
		int(contextCount),
	)

	for i := uint64(0); i < contextCount; i++ {
		var order uint8
		var key uint32
		var total uint32
		var symbolCount uint8

		if err := binary.Read(
			file,
			binary.BigEndian,
			&order,
		); err != nil {
			return nil, fmt.Errorf(
				"read context %d order: %w",
				i,
				err,
			)
		}

		if order > maxOrder {
			return nil, fmt.Errorf(
				"context %d has invalid order %d",
				i,
				order,
			)
		}

		if err := binary.Read(
			file,
			binary.BigEndian,
			&key,
		); err != nil {
			return nil, fmt.Errorf(
				"read context %d key: %w",
				i,
				err,
			)
		}

		if err := binary.Read(
			file,
			binary.BigEndian,
			&total,
		); err != nil {
			return nil, fmt.Errorf(
				"read context %d total: %w",
				i,
				err,
			)
		}

		if err := binary.Read(
			file,
			binary.BigEndian,
			&symbolCount,
		); err != nil {
			return nil, fmt.Errorf(
				"read context %d symbol count: %w",
				i,
				err,
			)
		}

		if symbolCount == 0 {
			return nil, fmt.Errorf(
				"context %d contains no symbols",
				i,
			)
		}

		context := &StaticPPMContext{
			Key:     key,
			Order:   order,
			Total:   total,
			Symbols: make([]StaticPPMSymbol, symbolCount),
		}

		var computedTotal uint32

		for j := 0; j < int(symbolCount); j++ {
			var symbol byte
			var count uint32

			if err := binary.Read(
				file,
				binary.BigEndian,
				&symbol,
			); err != nil {
				return nil, fmt.Errorf(
					"read context %d symbol %d: %w",
					i,
					j,
					err,
				)
			}

			if err := binary.Read(
				file,
				binary.BigEndian,
				&count,
			); err != nil {
				return nil, fmt.Errorf(
					"read context %d count %d: %w",
					i,
					j,
					err,
				)
			}

			if count == 0 {
				return nil, fmt.Errorf(
					"context %d contains zero-frequency symbol",
					i,
				)
			}

			context.Symbols[j] = StaticPPMSymbol{
				Symbol: symbol,
				Count:  count,
			}

			computedTotal += count
		}

		if computedTotal != total {
			return nil, fmt.Errorf(
				"context %d total mismatch: header=%d computed=%d",
				i,
				total,
				computedTotal,
			)
		}

		mapKey := staticPPMContextKey(order, key)

		if _, exists := model.contexts[mapKey]; exists {
			return nil, fmt.Errorf(
				"duplicate context: order=%d key=%d",
				order,
				key,
			)
		}

		model.contexts[mapKey] = context
	}

	return model, nil
}

func (m *StaticPPM) Context(
	order uint8,
	key uint32,
) *StaticPPMContext {
	return m.contexts[staticPPMContextKey(order, key)]
}

func (m *StaticPPM) ContextCount() int {
	return len(m.contexts)
}

func (m *StaticPPM) Lookup(
	history []byte,
	order int,
) *StaticPPMContext {
	if order < 0 || order > int(m.MaxOrder) {
		return nil
	}

	if order == 0 {
		return m.Context(0, 0)
	}

	if len(history) < order {
		return nil
	}

	start := len(history) - order

	var key uint32

	for i := start; i < len(history); i++ {
		key = (key << 8) | uint32(history[i])
	}

	return m.Context(uint8(order), key)
}

func (m *StaticPPM) BestContext(
	history []byte,
) (*StaticPPMContext, int) {
	maxOrder := len(history)

	if maxOrder > int(m.MaxOrder) {
		maxOrder = int(m.MaxOrder)
	}

	for order := maxOrder; order >= 0; order-- {
		context := m.Lookup(history, order)

		if context != nil && len(context.Symbols) > 0 {
			return context, order
		}
	}

	return nil, -1
}

func (c *StaticPPMContext) Find(
	symbol byte,
) (low uint32, high uint32, ok bool) {
	var cumulative uint32

	for _, entry := range c.Symbols {
		next := cumulative + entry.Count

		if entry.Symbol == symbol {
			return cumulative, next, true
		}

		cumulative = next
	}

	return 0, 0, false
}

func (c *StaticPPMContext) SymbolAt(
	value uint32,
) (symbol byte, low uint32, high uint32, ok bool) {
	if value >= c.Total {
		return 0, 0, 0, false
	}

	var cumulative uint32

	for _, entry := range c.Symbols {
		next := cumulative + entry.Count

		if value < next {
			return entry.Symbol, cumulative, next, true
		}

		cumulative = next
	}

	return 0, 0, 0, false
}
