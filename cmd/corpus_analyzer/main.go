package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "github.com/marcboeker/go-duckdb"
)

type Config struct {
	Input  string
	TopN   int
	Grams  int
	Output string
}

type Stats struct {
	TotalURLs     uint64 `json:"total_urls"`
	UniqueDomains uint64 `json:"unique_domains"`

	HTTPS uint64 `json:"https"`
	HTTP  uint64 `json:"http"`

	URLsWithPath     uint64 `json:"urls_with_path"`
	URLsWithQuery    uint64 `json:"urls_with_query"`
	URLsWithFragment uint64 `json:"urls_with_fragment"`

	MinLength    uint64  `json:"min_length"`
	MaxLength    uint64  `json:"max_length"`
	MeanLength   float64 `json:"mean_length"`
	MedianLength float64 `json:"median_length"`
	P90Length    float64 `json:"p90_length"`
	P95Length    float64 `json:"p95_length"`
	P99Length    float64 `json:"p99_length"`

	ByteEntropy float64 `json:"byte_entropy"`

	TopDomains      []Frequency `json:"top_domains"`
	TopPathSegments []Frequency `json:"top_path_segments"`
	TopQueryKeys    []Frequency `json:"top_query_keys"`
}

type Frequency struct {
	Value string `json:"value"`
	Count uint64 `json:"count"`
}

type Counter struct {
	mu sync.Mutex
	m  map[string]uint64
}

func NewCounter() *Counter {
	return &Counter{
		m: make(map[string]uint64),
	}
}

func (c *Counter) Add(s string) {
	if s == "" {
		return
	}

	c.mu.Lock()
	c.m[s]++
	c.mu.Unlock()
}

func (c *Counter) Top(n int) []Frequency {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]Frequency, 0, len(c.m))

	for value, count := range c.m {
		result = append(result, Frequency{
			Value: value,
			Count: count,
		})
	}

	sort.Slice(
		result,
		func(i, j int) bool {
			if result[i].Count != result[j].Count {
				return result[i].Count > result[j].Count
			}

			return result[i].Value < result[j].Value
		},
	)

	if len(result) > n {
		result = result[:n]
	}

	return result
}

type LengthStats struct {
	mu     sync.Mutex
	values []int
}

func (s *LengthStats) Add(v int) {
	s.mu.Lock()
	s.values = append(s.values, v)
	s.mu.Unlock()
}

func (s *LengthStats) Calculate() (
	min int,
	max int,
	mean float64,
	median float64,
	p90 float64,
	p95 float64,
	p99 float64,
) {
	s.mu.Lock()
	values := append([]int(nil), s.values...)
	s.mu.Unlock()

	if len(values) == 0 {
		return
	}

	sort.Ints(values)

	min = values[0]
	max = values[len(values)-1]

	var total uint64

	for _, v := range values {
		total += uint64(v)
	}

	mean = float64(total) / float64(len(values))

	percentile := func(p float64) float64 {
		if len(values) == 1 {
			return float64(values[0])
		}

		position := p * float64(len(values)-1)

		lower := int(math.Floor(position))
		upper := int(math.Ceil(position))

		if lower == upper {
			return float64(values[lower])
		}

		weight := position - float64(lower)

		return float64(values[lower]) +
			(float64(values[upper])-float64(values[lower]))*weight
	}

	median = percentile(0.50)
	p90 = percentile(0.90)
	p95 = percentile(0.95)
	p99 = percentile(0.99)

	return
}

type Histogram struct {
	mu sync.Mutex
	m  map[byte]uint64
}

func NewHistogram() *Histogram {
	return &Histogram{
		m: make(map[byte]uint64),
	}
}

func (h *Histogram) AddBytes(data string) {
	h.mu.Lock()

	for i := 0; i < len(data); i++ {
		h.m[data[i]]++
	}

	h.mu.Unlock()
}

func (h *Histogram) Entropy() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	var total uint64

	for _, count := range h.m {
		total += count
	}

	if total == 0 {
		return 0
	}

	var entropy float64

	for _, count := range h.m {
		p := float64(count) / float64(total)

		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func collectShardFiles(input string) ([]string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		if !strings.HasSuffix(input, ".parquet") ||
			strings.HasSuffix(input, ".tmp.parquet") {
			return nil, fmt.Errorf(
				"input is not a completed parquet shard: %s",
				input,
			)
		}

		return []string{input}, nil
	}

	// IMPORTANT:
	//
	// Only match:
	//
	//     shard-0000.parquet
	//
	// Do NOT match:
	//
	//     shard-0000.tmp.parquet
	//
	// because the collector may currently be writing that file.
	files, err := filepath.Glob(
		filepath.Join(
			input,
			"shard-[0-9][0-9][0-9][0-9].parquet",
		),
	)
	if err != nil {
		return nil, err
	}

	sort.Strings(files)

	if len(files) == 0 {
		return nil, fmt.Errorf(
			"no completed shard files found in %s",
			input,
		)
	}

	return files, nil
}

func buildParquetExpression(files []string) string {
	quoted := make([]string, len(files))

	for i, file := range files {
		quoted[i] = sqlQuote(file)
	}

	return "[" + strings.Join(quoted, ",") + "]"
}

func analyze(
	db *sql.DB,
	cfg Config,
	files []string,
) (*Stats, error) {
	stats := &Stats{}

	domainCounter := NewCounter()
	pathCounter := NewCounter()
	queryKeyCounter := NewCounter()

	byteHistogram := NewHistogram()
	lengthStats := &LengthStats{}

	parquet := buildParquetExpression(files)

	// ========================================================================
	// BASIC URL STATISTICS
	// ========================================================================

	log.Println("Collecting basic URL statistics...")

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) AS total_urls,

			COUNT(DISTINCT domain) AS unique_domains,

			COUNT(*) FILTER (
				WHERE lower(url_protocol) = 'https'
			) AS https,

			COUNT(*) FILTER (
				WHERE lower(url_protocol) = 'http'
			) AS http,

			COUNT(*) FILTER (
				WHERE url_path IS NOT NULL
				  AND url_path != ''
			) AS urls_with_path,

			COUNT(*) FILTER (
				WHERE url_query IS NOT NULL
				  AND url_query != ''
			) AS urls_with_query,

			COUNT(*) FILTER (
				WHERE position('#' IN url) > 0
			) AS urls_with_fragment

		FROM read_parquet(
			%s,
			union_by_name = true
		)
	`, parquet)

	var (
		totalURLs        uint64
		uniqueDomains    uint64
		https            uint64
		http             uint64
		urlsWithPath     uint64
		urlsWithQuery    uint64
		urlsWithFragment uint64
	)

	err := db.QueryRow(query).Scan(
		&totalURLs,
		&uniqueDomains,
		&https,
		&http,
		&urlsWithPath,
		&urlsWithQuery,
		&urlsWithFragment,
	)
	if err != nil {
		return nil, err
	}

	stats.TotalURLs = totalURLs
	stats.UniqueDomains = uniqueDomains
	stats.HTTPS = https
	stats.HTTP = http
	stats.URLsWithPath = urlsWithPath
	stats.URLsWithQuery = urlsWithQuery
	stats.URLsWithFragment = urlsWithFragment

	// ========================================================================
	// URL LENGTHS
	// ========================================================================

	log.Println("Collecting URL length distribution...")

	rows, err := db.Query(fmt.Sprintf(`
		SELECT length(url)
		FROM read_parquet(
			%s,
			union_by_name = true
		)
		WHERE url IS NOT NULL
	`, parquet))
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var length int

		if err := rows.Scan(&length); err != nil {
			rows.Close()
			return nil, err
		}

		lengthStats.Add(length)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	min, max, mean, median, p90, p95, p99 :=
		lengthStats.Calculate()

	stats.MinLength = uint64(min)
	stats.MaxLength = uint64(max)
	stats.MeanLength = mean
	stats.MedianLength = median
	stats.P90Length = p90
	stats.P95Length = p95
	stats.P99Length = p99

	// ========================================================================
	// DOMAIN FREQUENCIES
	// ========================================================================

	log.Println("Collecting domain frequencies...")

	rows, err = db.Query(fmt.Sprintf(`
		SELECT
			domain,
			COUNT(*)
		FROM read_parquet(
			%s,
			union_by_name = true
		)
		WHERE domain IS NOT NULL
		GROUP BY domain
	`, parquet))
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var (
			domain string
			count  uint64
		)

		if err := rows.Scan(&domain, &count); err != nil {
			rows.Close()
			return nil, err
		}

		domainCounter.m[domain] = count
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	stats.TopDomains = domainCounter.Top(
		cfg.TopN,
	)

	// ========================================================================
	// PATH SEGMENTS
	// ========================================================================

	log.Println("Collecting path segment frequencies...")

	rows, err = db.Query(fmt.Sprintf(`
		SELECT
			segment,
			COUNT(*)
		FROM (
			SELECT
				unnest(
					string_split(
						trim(url_path, '/'),
						'/'
					)
				) AS segment

			FROM read_parquet(
				%s,
				union_by_name = true
			)

			WHERE url_path IS NOT NULL
			  AND url_path != ''
		)

		WHERE segment != ''

		GROUP BY segment
	`, parquet))
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var (
			segment string
			count   uint64
		)

		if err := rows.Scan(&segment, &count); err != nil {
			rows.Close()
			return nil, err
		}

		pathCounter.m[segment] = count
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	stats.TopPathSegments = pathCounter.Top(
		cfg.TopN,
	)

	// ========================================================================
	// QUERY KEYS
	// ========================================================================

	log.Println("Collecting query-key frequencies...")

	rows, err = db.Query(fmt.Sprintf(`
		SELECT
			lower(
				split_part(parameter, '=', 1)
			) AS key,
			COUNT(*)
		FROM (
			SELECT
				unnest(
					string_split(
						url_query,
						'&'
					)
				) AS parameter

			FROM read_parquet(
				%s,
				union_by_name = true
			)

			WHERE url_query IS NOT NULL
			  AND url_query != ''
		)

		WHERE parameter != ''

		GROUP BY key
	`, parquet))
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var (
			key   string
			count uint64
		)

		if err := rows.Scan(&key, &count); err != nil {
			rows.Close()
			return nil, err
		}

		queryKeyCounter.m[key] = count
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	stats.TopQueryKeys = queryKeyCounter.Top(
		cfg.TopN,
	)

	// ========================================================================
	// BYTE DISTRIBUTION
	// ========================================================================

	log.Println("Collecting byte distribution...")

	rows, err = db.Query(fmt.Sprintf(`
		SELECT url
		FROM read_parquet(
			%s,
			union_by_name = true
		)
		WHERE url IS NOT NULL
	`, parquet))
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var url string

		if err := rows.Scan(&url); err != nil {
			rows.Close()
			return nil, err
		}

		byteHistogram.AddBytes(url)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	stats.ByteEntropy = byteHistogram.Entropy()

	return stats, nil
}

func printFrequencyTable(
	title string,
	values []Frequency,
) {
	fmt.Println()
	fmt.Println(title)
	fmt.Println(strings.Repeat("-", len(title)))

	for i, value := range values {
		fmt.Printf(
			"%4d  %-50s %12d\n",
			i+1,
			value.Value,
			value.Count,
		)
	}
}

func percentage(
	value uint64,
	total uint64,
) float64 {
	if total == 0 {
		return 0
	}

	return 100.0 *
		float64(value) /
		float64(total)
}

func printStats(stats *Stats) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("URL CORPUS ANALYSIS")
	fmt.Println("========================================")

	fmt.Printf(
		"Total URLs:          %d\n",
		stats.TotalURLs,
	)

	fmt.Printf(
		"Unique domains:      %d\n",
		stats.UniqueDomains,
	)

	fmt.Println()

	fmt.Println("PROTOCOL")
	fmt.Println("--------")

	fmt.Printf(
		"HTTPS:               %d (%.2f%%)\n",
		stats.HTTPS,
		percentage(stats.HTTPS, stats.TotalURLs),
	)

	fmt.Printf(
		"HTTP:                %d (%.2f%%)\n",
		stats.HTTP,
		percentage(stats.HTTP, stats.TotalURLs),
	)

	fmt.Println()

	fmt.Println("STRUCTURE")
	fmt.Println("---------")

	fmt.Printf(
		"With path:           %d (%.2f%%)\n",
		stats.URLsWithPath,
		percentage(
			stats.URLsWithPath,
			stats.TotalURLs,
		),
	)

	fmt.Printf(
		"With query:          %d (%.2f%%)\n",
		stats.URLsWithQuery,
		percentage(
			stats.URLsWithQuery,
			stats.TotalURLs,
		),
	)

	fmt.Printf(
		"With fragment:       %d (%.2f%%)\n",
		stats.URLsWithFragment,
		percentage(
			stats.URLsWithFragment,
			stats.TotalURLs,
		),
	)

	fmt.Println()

	fmt.Println("URL LENGTH")
	fmt.Println("----------")

	fmt.Printf(
		"Min:                 %d\n",
		stats.MinLength,
	)

	fmt.Printf(
		"Mean:                %.2f\n",
		stats.MeanLength,
	)

	fmt.Printf(
		"Median:              %.2f\n",
		stats.MedianLength,
	)

	fmt.Printf(
		"P90:                 %.2f\n",
		stats.P90Length,
	)

	fmt.Printf(
		"P95:                 %.2f\n",
		stats.P95Length,
	)

	fmt.Printf(
		"P99:                 %.2f\n",
		stats.P99Length,
	)

	fmt.Printf(
		"Max:                 %d\n",
		stats.MaxLength,
	)

	fmt.Println()

	fmt.Printf(
		"Byte entropy:        %.4f bits/byte\n",
		stats.ByteEntropy,
	)

	printFrequencyTable(
		"TOP DOMAINS",
		stats.TopDomains,
	)

	printFrequencyTable(
		"TOP PATH SEGMENTS",
		stats.TopPathSegments,
	)

	printFrequencyTable(
		"TOP QUERY KEYS",
		stats.TopQueryKeys,
	)
}

func writeJSON(
	path string,
	stats *Stats,
) error {
	data, err := json.MarshalIndent(
		stats,
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	return os.WriteFile(
		path,
		data,
		0644,
	)
}

func main() {
	cfg := Config{}

	flag.StringVar(
		&cfg.Input,
		"input",
		"corpus/raw/tranco_shards",
		"Parquet shard directory or Parquet file",
	)

	flag.IntVar(
		&cfg.TopN,
		"top",
		100,
		"Number of top frequencies to display",
	)

	flag.IntVar(
		&cfg.Grams,
		"grams",
		4,
		"Maximum n-gram size reserved for future analysis",
	)

	flag.StringVar(
		&cfg.Output,
		"output",
		"",
		"Optional JSON output path",
	)

	flag.Parse()

	if cfg.TopN <= 0 {
		log.Fatal("--top must be greater than zero")
	}

	if cfg.Grams <= 0 {
		log.Fatal("--grams must be greater than zero")
	}

	files, err := collectShardFiles(
		cfg.Input,
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf(
		"Found %d completed parquet shard(s)",
		len(files),
	)

	db, err := sql.Open(
		"duckdb",
		"",
	)
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	stats, err := analyze(
		db,
		cfg,
		files,
	)
	if err != nil {
		log.Fatal(err)
	}

	printStats(stats)

	if cfg.Output != "" {
		if err := writeJSON(
			cfg.Output,
			stats,
		); err != nil {
			log.Fatal(err)
		}

		log.Printf(
			"\nJSON written to %s",
			cfg.Output,
		)
	}
}
