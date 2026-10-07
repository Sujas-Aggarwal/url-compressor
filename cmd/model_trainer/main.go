package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

type Counter map[string]uint64

type Token struct {
	Value string `json:"value"`
	Count uint64 `json:"count"`
}

type Model struct {
	Version int `json:"version"`

	DomainLabels []Token `json:"domain_labels"`
	PathSegments []Token `json:"path_segments"`
	QueryKeys    []Token `json:"query_keys"`

	Stats ModelStats `json:"stats"`
}

type ModelStats struct {
	URLs         uint64 `json:"urls"`
	DomainLabels uint64 `json:"domain_labels"`
	PathSegments uint64 `json:"path_segments"`
	QueryKeys    uint64 `json:"query_keys"`
}

func increment(m Counter, value string) {
	if value == "" {
		return
	}

	m[value]++
}

func topTokens(
	counter Counter,
	limit int,
	minCount uint64,
) []Token {
	tokens := make([]Token, 0, len(counter))

	for value, count := range counter {
		if count < minCount {
			continue
		}

		tokens = append(tokens, Token{
			Value: value,
			Count: count,
		})
	}

	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].Count != tokens[j].Count {
			return tokens[i].Count > tokens[j].Count
		}

		return tokens[i].Value < tokens[j].Value
	})

	if len(tokens) > limit {
		tokens = tokens[:limit]
	}

	return tokens
}

func splitPath(path string) []string {
	if path == "" || path == "/" {
		return nil
	}

	path = strings.TrimPrefix(path, "/")

	parts := strings.Split(path, "/")

	result := make([]string, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		result = append(result, part)
	}

	return result
}

func processURL(
	raw string,
	domain Counter,
	path Counter,
	query Counter,
) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}

	// ------------------------------------------------------------
	// Domain labels
	// ------------------------------------------------------------

	hostname := strings.ToLower(parsed.Hostname())

	if hostname != "" {
		for _, label := range strings.Split(hostname, ".") {
			increment(domain, label)
		}
	}

	// ------------------------------------------------------------
	// Path segments
	// ------------------------------------------------------------

	for _, segment := range splitPath(parsed.EscapedPath()) {
		increment(path, segment)
	}

	// ------------------------------------------------------------
	// Query keys
	// ------------------------------------------------------------

	// RawQuery is used instead of parsed.Query() so that we don't
	// unnecessarily decode values and create huge allocations.
	if parsed.RawQuery != "" {
		for _, parameter := range strings.Split(parsed.RawQuery, "&") {
			if parameter == "" {
				continue
			}

			key := parameter

			if index := strings.IndexByte(parameter, '='); index >= 0 {
				key = parameter[:index]
			}

			if key == "" {
				continue
			}

			// Query keys are normalized only enough to make the
			// dictionary case-insensitive.
			increment(query, strings.ToLower(key))
		}
	}

	return true
}

func main() {
	input := flag.String(
		"input",
		"corpus/training/urls.txt",
		"Training corpus",
	)

	output := flag.String(
		"output",
		"corpus/model/model.json",
		"Output model",
	)

	domainLimit := flag.Int(
		"domain-limit",
		65536,
		"Maximum domain-label vocabulary size",
	)

	pathLimit := flag.Int(
		"path-limit",
		65536,
		"Maximum path-segment vocabulary size",
	)

	queryLimit := flag.Int(
		"query-limit",
		16384,
		"Maximum query-key vocabulary size",
	)

	minCount := flag.Uint64(
		"min-count",
		20,
		"Minimum token frequency",
	)

	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("URL MODEL TRAINER")
	fmt.Println("========================================")
	fmt.Printf("Input:          %s\n", *input)
	fmt.Printf("Output:         %s\n", *output)
	fmt.Printf("Domain limit:   %d\n", *domainLimit)
	fmt.Printf("Path limit:     %d\n", *pathLimit)
	fmt.Printf("Query limit:    %d\n", *queryLimit)
	fmt.Printf("Minimum count:  %d\n", *minCount)
	fmt.Println()

	file, err := os.Open(*input)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	domainCounts := make(Counter)
	pathCounts := make(Counter)
	queryCounts := make(Counter)

	scanner := bufio.NewScanner(file)

	// URLs can be up to several KB.
	scanner.Buffer(
		make([]byte, 64*1024),
		8*1024*1024,
	)

	var urls uint64

	for scanner.Scan() {
		raw := scanner.Text()

		if processURL(
			raw,
			domainCounts,
			pathCounts,
			queryCounts,
		) {
			urls++
		}

		if urls%1_000_000 == 0 {
			fmt.Printf(
				"processed %,d URLs | domain=%d path=%d query=%d\n",
				urls,
				len(domainCounts),
				len(pathCounts),
				len(queryCounts),
			)
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("Building vocabularies...")

	domainTokens := topTokens(
		domainCounts,
		*domainLimit,
		*minCount,
	)

	pathTokens := topTokens(
		pathCounts,
		*pathLimit,
		*minCount,
	)

	queryTokens := topTokens(
		queryCounts,
		*queryLimit,
		*minCount,
	)

	model := Model{
		Version: 1,

		DomainLabels: domainTokens,
		PathSegments: pathTokens,
		QueryKeys:    queryTokens,

		Stats: ModelStats{
			URLs:         urls,
			DomainLabels: uint64(len(domainCounts)),
			PathSegments: uint64(len(pathCounts)),
			QueryKeys:    uint64(len(queryCounts)),
		},
	}

	if err := os.MkdirAll(
		"corpus/model",
		0755,
	); err != nil {
		panic(err)
	}

	data, err := json.MarshalIndent(
		model,
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

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("MODEL TRAINED")
	fmt.Println("========================================")
	fmt.Printf("URLs:              %,d\n", urls)
	fmt.Printf("Unique domain:     %,d\n", len(domainCounts))
	fmt.Printf("Domain vocabulary: %,d\n", len(domainTokens))
	fmt.Printf("Unique paths:      %,d\n", len(pathCounts))
	fmt.Printf("Path vocabulary:   %,d\n", len(pathTokens))
	fmt.Printf("Unique query keys:  %,d\n", len(queryCounts))
	fmt.Printf("Query vocabulary:  %,d\n", len(queryTokens))
	fmt.Println()
	fmt.Printf("Model: %s\n", *output)
}
