package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
)

const Alphabet = 256

type NGramStats struct {
	Counts [256]uint64

	// For order 1:
	//   context byte -> next byte counts
	Bigram [256][256]uint64

	// For order 2+ we use sparse maps.
	// key = packed context + next byte.
	Trigram  map[uint64]uint64
	Fourgram map[uint64]uint64
	Fivegram map[uint64]uint64

	Total uint64
}

type Result struct {
	TotalBytes uint64 `json:"total_bytes"`

	Entropy struct {
		Order0 float64 `json:"H(X)"`
		Order1 float64 `json:"H(X|X-1)"`
		Order2 float64 `json:"H(X|X-2,X-1)"`
		Order3 float64 `json:"H(X|X-3,X-2,X-1)"`
		Order4 float64 `json:"H(X|X-4,X-3,X-2,X-1)"`
	} `json:"entropy"`

	Distinct struct {
		Unigrams  uint64 `json:"unigrams"`
		Bigrams   uint64 `json:"bigrams"`
		Trigrams  uint64 `json:"trigrams"`
		Fourgrams uint64 `json:"fourgrams"`
		Fivegrams uint64 `json:"fivegrams"`
	} `json:"distinct"`
}

func entropyFromCounts(counts []uint64, total uint64) float64 {
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

func conditionalEntropy(
	ngram map[uint64]uint64,
	contextBits uint,
) float64 {
	if len(ngram) == 0 {
		return 0
	}

	// Aggregate total occurrences for each context.
	contextTotals := make(map[uint64]uint64)

	for key, count := range ngram {
		context := key >> 8
		contextTotals[context] += count
	}

	var result float64

	for key, count := range ngram {
		context := key >> 8
		total := contextTotals[context]

		p := float64(count) / float64(total)

		result -=
			(float64(total) / float64(sumMap(contextTotals))) *
				p *
				math.Log2(p)
	}

	return result
}

func sumMap(m map[uint64]uint64) uint64 {
	var total uint64

	for _, v := range m {
		total += v
	}

	return total
}

func pack(context uint64, next byte) uint64 {
	return (context << 8) | uint64(next)
}

func analyzeLine(
	stats *NGramStats,
	line []byte,
) {
	// Do not include the newline.
	if len(line) == 0 {
		return
	}

	for _, b := range line {
		stats.Counts[b]++
		stats.Total++
	}

	// Bigram
	for i := 1; i < len(line); i++ {
		prev := line[i-1]
		next := line[i]

		stats.Bigram[prev][next]++
	}

	// Trigram
	for i := 2; i < len(line); i++ {
		context :=
			(uint64(line[i-2]) << 8) |
				uint64(line[i-1])

		stats.Trigram[pack(context, line[i])]++
	}

	// Fourgram
	for i := 3; i < len(line); i++ {
		context :=
			(uint64(line[i-3]) << 16) |
				(uint64(line[i-2]) << 8) |
				uint64(line[i-1])

		stats.Fourgram[pack(context, line[i])]++
	}

	// Fivegram
	for i := 4; i < len(line); i++ {
		context :=
			(uint64(line[i-4]) << 24) |
				(uint64(line[i-3]) << 16) |
				(uint64(line[i-2]) << 8) |
				uint64(line[i-1])

		stats.Fivegram[pack(context, line[i])]++
	}
}

func bigramEntropy(stats *NGramStats) float64 {
	var result float64

	for context := 0; context < 256; context++ {
		total := uint64(0)

		for next := 0; next < 256; next++ {
			total += stats.Bigram[context][next]
		}

		if total == 0 {
			continue
		}

		for next := 0; next < 256; next++ {
			count := stats.Bigram[context][next]

			if count == 0 {
				continue
			}

			pContext := float64(total) / float64(stats.Total-1)
			p := float64(count) / float64(total)

			result -= pContext * p * math.Log2(p)
		}
	}

	return result
}

func mapConditionalEntropy(
	ngram map[uint64]uint64,
	total uint64,
) float64 {
	if len(ngram) == 0 || total == 0 {
		return 0
	}

	contextTotals := make(map[uint64]uint64)

	for key, count := range ngram {
		context := key >> 8
		contextTotals[context] += count
	}

	var h float64

	for key, count := range ngram {
		context := key >> 8
		contextTotal := contextTotals[context]

		if contextTotal == 0 {
			continue
		}

		pContext := float64(contextTotal) / float64(total)
		p := float64(count) / float64(contextTotal)

		h -= pContext * p * math.Log2(p)
	}

	return h
}

func main() {
	input := flag.String(
		"input",
		"corpus/training/urls.txt",
		"Training corpus",
	)

	output := flag.String(
		"output",
		"corpus/training/ngram_analysis.json",
		"JSON output",
	)

	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("URL N-GRAM ANALYZER")
	fmt.Println("========================================")
	fmt.Printf("Input: %s\n\n", *input)

	file, err := os.Open(*input)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	stats := &NGramStats{
		Trigram:  make(map[uint64]uint64),
		Fourgram: make(map[uint64]uint64),
		Fivegram: make(map[uint64]uint64),
	}

	scanner := bufio.NewScanner(file)

	// URLs can be up to several KB.
	scanner.Buffer(
		make([]byte, 64*1024),
		8*1024*1024,
	)

	var lines uint64

	for scanner.Scan() {
		line := scanner.Bytes()

		// Copy because Scanner reuses its buffer.
		data := append([]byte(nil), line...)

		analyzeLine(stats, data)

		lines++

		if lines%1_000_000 == 0 {
			fmt.Printf(
				"processed %,d URLs | %,d bytes\n",
				lines,
				stats.Total,
			)
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("Computing entropy...")

	result := Result{
		TotalBytes: stats.Total,
	}

	result.Entropy.Order0 =
		entropyFromCounts(
			stats.Counts[:],
			stats.Total,
		)

	result.Entropy.Order1 =
		bigramEntropy(stats)

	// H(X_i | X_{i-2}, X_{i-1})
	result.Entropy.Order2 =
		mapConditionalEntropy(
			stats.Trigram,
			stats.Total-2,
		)

	// H(X_i | X_{i-3}, X_{i-2}, X_{i-1})
	result.Entropy.Order3 =
		mapConditionalEntropy(
			stats.Fourgram,
			stats.Total-3,
		)

	// H(X_i | X_{i-4}, X_{i-3}, X_{i-2}, X_{i-1})
	result.Entropy.Order4 =
		mapConditionalEntropy(
			stats.Fivegram,
			stats.Total-4,
		)

	for _, count := range stats.Counts {
		if count > 0 {
			result.Distinct.Unigrams++
		}
	}

	var bigrams uint64
	for i := range stats.Bigram {
		for j := range stats.Bigram[i] {
			if stats.Bigram[i][j] > 0 {
				bigrams++
			}
		}
	}

	result.Distinct.Bigrams = bigrams
	result.Distinct.Trigrams = uint64(len(stats.Trigram))
	result.Distinct.Fourgrams = uint64(len(stats.Fourgram))
	result.Distinct.Fivegrams = uint64(len(stats.Fivegram))

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("RESULT")
	fmt.Println("========================================")

	fmt.Printf("Total bytes: %,d\n", result.TotalBytes)
	fmt.Println()

	fmt.Println("ENTROPY")
	fmt.Println("----------------------------------------")
	fmt.Printf("H(X)                  %.4f bits/byte\n",
		result.Entropy.Order0)
	fmt.Printf("H(X | X-1)            %.4f bits/byte\n",
		result.Entropy.Order1)
	fmt.Printf("H(X | X-2,X-1)        %.4f bits/byte\n",
		result.Entropy.Order2)
	fmt.Printf("H(X | X-3..X-1)       %.4f bits/byte\n",
		result.Entropy.Order3)
	fmt.Printf("H(X | X-4..X-1)       %.4f bits/byte\n",
		result.Entropy.Order4)

	fmt.Println()
	fmt.Println("DISTINCT N-GRAMS")
	fmt.Println("----------------------------------------")
	fmt.Printf("Unigrams:   %,d\n",
		result.Distinct.Unigrams)
	fmt.Printf("Bigrams:    %,d\n",
		result.Distinct.Bigrams)
	fmt.Printf("Trigrams:   %,d\n",
		result.Distinct.Trigrams)
	fmt.Printf("Fourgrams:  %,d\n",
		result.Distinct.Fourgrams)
	fmt.Printf("Fivegrams:  %,d\n",
		result.Distinct.Fivegrams)

	if *output != "" {
		data, err := json.MarshalIndent(
			result,
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

	// Silence unused import warning if strings is removed
	_ = strings.Builder{}
}
