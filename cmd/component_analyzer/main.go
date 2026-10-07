package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"strings"
)

type ComponentStats struct {
	Counts [256]uint64

	Bigram   [256][256]uint64
	Trigram  map[uint64]uint64
	Fourgram map[uint64]uint64
	Fivegram map[uint64]uint64

	Total uint64
}

type EntropyResult struct {
	Bytes uint64 `json:"bytes"`

	H0 float64 `json:"H(X)"`
	H1 float64 `json:"H(X|X-1)"`
	H2 float64 `json:"H(X|X-2,X-1)"`
	H3 float64 `json:"H(X|X-3,X-2,X-1)"`
	H4 float64 `json:"H(X|X-4,X-3,X-2,X-1)"`
}

type Report struct {
	URLs uint64 `json:"urls"`

	Components map[string]EntropyResult `json:"components"`
}

func pack(context uint64, next byte) uint64 {
	return (context << 8) | uint64(next)
}

func addBytes(stats *ComponentStats, data []byte) {
	if len(data) == 0 {
		return
	}

	for _, b := range data {
		stats.Counts[b]++
		stats.Total++
	}

	for i := 1; i < len(data); i++ {
		stats.Bigram[data[i-1]][data[i]]++
	}

	for i := 2; i < len(data); i++ {
		context :=
			(uint64(data[i-2]) << 8) |
				uint64(data[i-1])

		stats.Trigram[pack(context, data[i])]++
	}

	for i := 3; i < len(data); i++ {
		context :=
			(uint64(data[i-3]) << 16) |
				(uint64(data[i-2]) << 8) |
				uint64(data[i-1])

		stats.Fourgram[pack(context, data[i])]++
	}

	for i := 4; i < len(data); i++ {
		context :=
			(uint64(data[i-4]) << 24) |
				(uint64(data[i-3]) << 16) |
				(uint64(data[i-2]) << 8) |
				uint64(data[i-1])

		stats.Fivegram[pack(context, data[i])]++
	}
}

func newStats() *ComponentStats {
	return &ComponentStats{
		Trigram:  make(map[uint64]uint64),
		Fourgram: make(map[uint64]uint64),
		Fivegram: make(map[uint64]uint64),
	}
}

func entropy(counts [256]uint64, total uint64) float64 {
	if total == 0 {
		return 0
	}

	var h float64

	for _, count := range counts {
		if count == 0 {
			continue
		}

		p := float64(count) / float64(total)
		h -= p * math.Log2(p)
	}

	return h
}

func bigramEntropy(stats *ComponentStats) float64 {
	if stats.Total < 2 {
		return 0
	}

	totalTransitions := stats.Total - 1

	var h float64

	for context := 0; context < 256; context++ {
		var contextTotal uint64

		for next := 0; next < 256; next++ {
			contextTotal += stats.Bigram[context][next]
		}

		if contextTotal == 0 {
			continue
		}

		pContext :=
			float64(contextTotal) /
				float64(totalTransitions)

		for next := 0; next < 256; next++ {
			count := stats.Bigram[context][next]

			if count == 0 {
				continue
			}

			p := float64(count) / float64(contextTotal)

			h -= pContext * p * math.Log2(p)
		}
	}

	return h
}

func mapConditionalEntropy(
	ngrams map[uint64]uint64,
	total uint64,
) float64 {
	if len(ngrams) == 0 || total == 0 {
		return 0
	}

	contextTotals := make(map[uint64]uint64)

	for key, count := range ngrams {
		context := key >> 8
		contextTotals[context] += count
	}

	var h float64

	for key, count := range ngrams {
		context := key >> 8
		contextTotal := contextTotals[context]

		if contextTotal == 0 {
			continue
		}

		pContext :=
			float64(contextTotal) /
				float64(total)

		p := float64(count) /
			float64(contextTotal)

		h -= pContext * p * math.Log2(p)
	}

	return h
}

func calculate(stats *ComponentStats) EntropyResult {
	result := EntropyResult{
		Bytes: stats.Total,
	}

	result.H0 = entropy(
		stats.Counts,
		stats.Total,
	)

	result.H1 = bigramEntropy(stats)

	if stats.Total >= 3 {
		result.H2 = mapConditionalEntropy(
			stats.Trigram,
			stats.Total-2,
		)
	}

	if stats.Total >= 4 {
		result.H3 = mapConditionalEntropy(
			stats.Fourgram,
			stats.Total-3,
		)
	}

	if stats.Total >= 5 {
		result.H4 = mapConditionalEntropy(
			stats.Fivegram,
			stats.Total-4,
		)
	}

	return result
}

func main() {
	input := flag.String(
		"input",
		"corpus/training/urls.txt",
		"Training corpus",
	)

	output := flag.String(
		"output",
		"corpus/training/component_analysis.json",
		"JSON output",
	)

	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("URL COMPONENT ENTROPY ANALYZER")
	fmt.Println("========================================")
	fmt.Printf("Input: %s\n\n", *input)

	file, err := os.Open(*input)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	stats := map[string]*ComponentStats{
		"whole_url":   newStats(),
		"scheme":      newStats(),
		"domain":      newStats(),
		"path":        newStats(),
		"query_key":   newStats(),
		"query_value": newStats(),
		"fragment":    newStats(),
	}

	scanner := bufio.NewScanner(file)

	scanner.Buffer(
		make([]byte, 64*1024),
		8*1024*1024,
	)

	var urls uint64

	for scanner.Scan() {
		raw := scanner.Text()

		// Whole URL.
		addBytes(
			stats["whole_url"],
			[]byte(raw),
		)

		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}

		// Scheme.
		addBytes(
			stats["scheme"],
			[]byte(parsed.Scheme),
		)

		// Hostname only.
		addBytes(
			stats["domain"],
			[]byte(parsed.Hostname()),
		)

		// Path.
		addBytes(
			stats["path"],
			[]byte(parsed.EscapedPath()),
		)

		// Query.
		query := parsed.Query()

		for key, values := range query {
			addBytes(
				stats["query_key"],
				[]byte(key),
			)

			for _, value := range values {
				addBytes(
					stats["query_value"],
					[]byte(value),
				)
			}
		}

		// Fragment.
		addBytes(
			stats["fragment"],
			[]byte(parsed.Fragment),
		)

		urls++

		if urls%1_000_000 == 0 {
			fmt.Printf(
				"processed %,d URLs\n",
				urls,
			)
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	report := Report{
		URLs:       urls,
		Components: make(map[string]EntropyResult),
	}

	order := []string{
		"whole_url",
		"scheme",
		"domain",
		"path",
		"query_key",
		"query_value",
		"fragment",
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("RESULT")
	fmt.Println("========================================")

	for _, name := range order {
		result := calculate(stats[name])
		report.Components[name] = result

		fmt.Printf("\n%-12s\n", strings.ToUpper(name))
		fmt.Println("----------------------------------------")
		fmt.Printf(
			"bytes: %,d\n",
			result.Bytes,
		)
		fmt.Printf(
			"H(X):       %.4f bits/byte\n",
			result.H0,
		)
		fmt.Printf(
			"H(X|1):     %.4f bits/byte\n",
			result.H1,
		)
		fmt.Printf(
			"H(X|2):     %.4f bits/byte\n",
			result.H2,
		)
		fmt.Printf(
			"H(X|3):     %.4f bits/byte\n",
			result.H3,
		)
		fmt.Printf(
			"H(X|4):     %.4f bits/byte\n",
			result.H4,
		)
	}

	data, err := json.MarshalIndent(
		report,
		"",
		"  ",
	)
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(
		*output,
		data,
		0644,
	); err != nil {
		panic(err)
	}

	fmt.Printf(
		"\nAnalysis written to %s\n",
		*output,
	)
}
