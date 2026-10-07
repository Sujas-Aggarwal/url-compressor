package codec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStaticPPM(t *testing.T) {
	path := filepath.Join(
		"../../",
		"corpus",
		"model",
		"ppm3.bin",
	)

	if _, err := os.Stat(path); err != nil {
		t.Skipf("trained model not available: %v", err)
	}

	model, err := LoadStaticPPM(path)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}

	if model.MaxOrder != 3 {
		t.Fatalf(
			"max order: got %d want 3",
			model.MaxOrder,
		)
	}

	if model.ContextCount() != 357954 {
		t.Fatalf(
			"context count: got %d want 357954",
			model.ContextCount(),
		)
	}

	order0 := model.Context(0, 0)

	if order0 == nil {
		t.Fatal("missing order-0 context")
	}

	if order0.Total == 0 {
		t.Fatal("order-0 context has zero total")
	}

	if len(order0.Symbols) == 0 {
		t.Fatal("order-0 context has no symbols")
	}

	t.Logf(
		"loaded %d contexts, order-0 total=%d symbols=%d",
		model.ContextCount(),
		order0.Total,
		len(order0.Symbols),
	)
}

func TestStaticPPMLookup(t *testing.T) {
	model := NewStaticPPM()

	model.MaxOrder = 3

	model.contexts[staticPPMContextKey(0, 0)] = &StaticPPMContext{
		Key:   0,
		Order: 0,
		Total: 10,
		Symbols: []StaticPPMSymbol{
			{Symbol: 'a', Count: 6},
			{Symbol: 'b', Count: 4},
		},
	}

	model.contexts[staticPPMContextKey(1, 'a')] = &StaticPPMContext{
		Key:   uint32('a'),
		Order: 1,
		Total: 10,
		Symbols: []StaticPPMSymbol{
			{Symbol: 'b', Count: 7},
			{Symbol: 'c', Count: 3},
		},
	}

	context := model.Lookup(
		[]byte("a"),
		1,
	)

	if context == nil {
		t.Fatal("expected context")
	}

	if context.Order != 1 {
		t.Fatalf(
			"order: got %d want 1",
			context.Order,
		)
	}

	low, high, ok := context.Find('b')

	if !ok {
		t.Fatal("expected b")
	}

	if low != 0 || high != 7 {
		t.Fatalf(
			"b interval: got [%d,%d), want [0,7)",
			low,
			high,
		)
	}

	symbol, low, high, ok := context.SymbolAt(8)

	if !ok {
		t.Fatal("expected symbol at cumulative 8")
	}

	if symbol != 'c' {
		t.Fatalf(
			"symbol: got %q want %q",
			symbol,
			'c',
		)
	}

	if low != 7 || high != 10 {
		t.Fatalf(
			"c interval: got [%d,%d), want [7,10)",
			low,
			high,
		)
	}
}
