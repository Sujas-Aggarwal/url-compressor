package codec

import (
	"testing"
)

type testSymbol struct {
	low   uint64
	high  uint64
	total uint64
}

func TestRangeCodec(t *testing.T) {
	tests := []struct {
		name string
		seq  []testSymbol
	}{
		{
			name: "single symbol",
			seq: []testSymbol{
				{0, 1, 1},
			},
		},
		{
			name: "binary",
			seq: []testSymbol{
				{0, 1, 2},
				{1, 2, 2},
				{1, 2, 2},
				{0, 1, 2},
				{0, 1, 2},
				{1, 2, 2},
			},
		},
		{
			name: "skewed distribution",
			seq: []testSymbol{
				{0, 900, 1000},
				{900, 950, 1000},
				{0, 900, 1000},
				{950, 1000, 1000},
				{0, 900, 1000},
			},
		},
		{
			name: "large total",
			seq: []testSymbol{
				{123, 456789, 1000000},
				{0, 123, 1000000},
				{456789, 900000, 1000000},
				{900000, 1000000, 1000000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder := NewRangeEncoder()

			for _, s := range tt.seq {
				if err := encoder.Encode(
					s.low,
					s.high,
					s.total,
				); err != nil {
					t.Fatalf("encode: %v", err)
				}
			}

			data := encoder.Finish()

			decoder, err := NewRangeDecoder(data)
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}

			for i, s := range tt.seq {
				value, err := decoder.Get(s.total)
				if err != nil {
					t.Fatalf(
						"symbol %d get: %v",
						i,
						err,
					)
				}

				if value < s.low || value >= s.high {
					t.Fatalf(
						"symbol %d: decoded value %d outside [%d,%d)",
						i,
						value,
						s.low,
						s.high,
					)
				}

				if err := decoder.Decode(
					s.low,
					s.high,
					s.total,
				); err != nil {
					t.Fatalf(
						"symbol %d decode: %v",
						i,
						err,
					)
				}
			}
		})
	}
}

func TestRangeCodecAllSymbols(t *testing.T) {
	const total = 256

	encoder := NewRangeEncoder()

	for i := 0; i < 256; i++ {
		if err := encoder.Encode(
			uint64(i),
			uint64(i+1),
			total,
		); err != nil {
			t.Fatalf("encode %d: %v", i, err)
		}
	}

	data := encoder.Finish()

	decoder, err := NewRangeDecoder(data)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}

	for i := 0; i < 256; i++ {
		value, err := decoder.Get(total)
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}

		if value != uint64(i) {
			t.Fatalf(
				"symbol %d: got %d",
				i,
				value,
			)
		}

		if err := decoder.Decode(
			uint64(i),
			uint64(i+1),
			total,
		); err != nil {
			t.Fatalf("decode %d: %v", i, err)
		}
	}
}
