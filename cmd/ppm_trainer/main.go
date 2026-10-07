package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
)

type countEntry struct {
	context uint64
	symbol  byte
	count   uint32
}

type modelContext struct {
	key     uint32
	order   uint8
	symbols []countEntry
	total   uint32
}

func contextID(order int, history []byte) uint32 {
	var key uint32

	start := len(history) - order

	for i := start; i < len(history); i++ {
		key = (key << 8) | uint32(history[i])
	}

	return key
}

func packedContext(order int, key uint32) uint64 {
	return (uint64(uint32(order)) << 32) | uint64(key)
}

func splitTrain(url []byte, percent int) bool {
	hash := sha256.Sum256(url)

	n := binary.BigEndian.Uint32(hash[:4]) % 100

	return int(n) < percent
}

func main() {
	input := flag.String(
		"input",
		"corpus/training/urls.txt",
		"training corpus",
	)

	output := flag.String(
		"output",
		"corpus/model/ppm3.bin",
		"output model",
	)

	maxOrder := flag.Int(
		"max-order",
		3,
		"maximum PPM order",
	)

	minCount := flag.Uint(
		"min-count",
		2,
		"minimum symbol count to retain",
	)

	topK := flag.Int(
		"top-k",
		16,
		"maximum symbols retained per context",
	)

	trainPercent := flag.Int(
		"train-percent",
		90,
		"percentage of URLs used for training",
	)

	progress := flag.Int64(
		"progress",
		100000,
		"print progress every N URLs",
	)

	flag.Parse()

	if *maxOrder < 0 || *maxOrder > 3 {
		panic("max-order must be between 0 and 3")
	}

	if *topK < 1 || *topK > 255 {
		panic("top-k must be between 1 and 255")
	}

	if *trainPercent < 1 || *trainPercent > 100 {
		panic("train-percent must be between 1 and 100")
	}

	file, err := os.Open(*input)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 1024*1024)

	// Key:
	//
	//   [order: 32 bits][context: 32 bits][symbol: 8 bits]
	//
	// This stores sparse counts without allocating a map for every context.
	counts := make(map[uint64]uint32, 2_000_000)

	var (
		urls       int64
		trainURLs  int64
		totalBytes int64
	)

	for {
		line, err := reader.ReadBytes('\n')

		if err != nil && err != io.EOF {
			panic(err)
		}

		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}

		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) > 0 {
			urls++

			if splitTrain(line, *trainPercent) {
				trainURLs++
				totalBytes += int64(len(line))

				history := make([]byte, 0, *maxOrder)

				for _, symbol := range line {
					for order := 0; order <= *maxOrder; order++ {
						if order > len(history) {
							break
						}

						var context uint32

						if order > 0 {
							context = contextID(order, history)
						}

						key :=
							(packedContext(order, context) << 8) |
								uint64(symbol)

						counts[key]++
					}

					history = append(history, symbol)

					if len(history) > *maxOrder {
						history = history[1:]
					}
				}
			}
		}

		if *progress > 0 && urls%*progress == 0 {
			fmt.Printf(
				"urls=%d train=%d bytes=%d entries=%d\n",
				urls,
				trainURLs,
				totalBytes,
				len(counts),
			)
		}

		if err == io.EOF {
			break
		}
	}

	fmt.Println()
	fmt.Println("training complete")
	fmt.Printf("urls:        %d\n", urls)
	fmt.Printf("train urls:  %d\n", trainURLs)
	fmt.Printf("train bytes: %d\n", totalBytes)
	fmt.Printf("raw entries: %d\n", len(counts))

	/*
		Convert sparse map into sortable entries.

		We remove low-frequency symbols here.

		Order 0 is special:
		we retain every observed byte because it is our final
		fallback context.
	*/
	entries := make([]countEntry, 0, len(counts))

	for key, count := range counts {
		context := key >> 8
		order := uint8(context >> 32)

		if order != 0 && count < uint32(*minCount) {
			continue
		}

		entries = append(entries, countEntry{
			context: context,
			symbol:  byte(key),
			count:   count,
		})
	}

	/*
		Sort by:

		  context
		  count descending
		  symbol ascending

		This makes the model deterministic.
	*/
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].context != entries[j].context {
			return entries[i].context < entries[j].context
		}

		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}

		return entries[i].symbol < entries[j].symbol
	})

	contexts := make([]modelContext, 0)

	for i := 0; i < len(entries); {
		context := entries[i].context
		order := uint8(context >> 32)

		j := i

		for j < len(entries) &&
			entries[j].context == context {
			j++
		}

		group := entries[i:j]

		/*
			Order 0 keeps every observed symbol.

			Higher-order contexts keep only the top K symbols.
		*/
		if order != 0 && len(group) > *topK {
			group = group[:*topK]
		}

		model := modelContext{
			key:     uint32(context),
			order:   order,
			symbols: append([]countEntry(nil), group...),
		}

		for _, entry := range model.symbols {
			model.total += entry.count
		}

		if model.total > 0 {
			contexts = append(contexts, model)
		}

		i = j
	}

	sort.Slice(contexts, func(i, j int) bool {
		if contexts[i].order != contexts[j].order {
			return contexts[i].order < contexts[j].order
		}

		return contexts[i].key < contexts[j].key
	})

	if err := os.MkdirAll(dir(*output), 0755); err != nil {
		panic(err)
	}

	out, err := os.Create(*output)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	/*
		Binary format:

		Header:

		    magic          4 bytes   "PPM3"
		    version        1 byte
		    maxOrder       1 byte
		    trainPercent   1 byte
		    reserved       1 byte
		    totalURLs      8 bytes
		    trainURLs      8 bytes
		    contextCount   8 bytes

		Each context:

		    contextKey     4 bytes
		    total          4 bytes
		    symbolCount    1 byte

		Each symbol:

		    symbol         1 byte
		    count          4 bytes
	*/

	if _, err := out.Write([]byte{'P', 'P', 'M', '3'}); err != nil {
		panic(err)
	}

	write := func(value any) {
		if err := binary.Write(
			out,
			binary.BigEndian,
			value,
		); err != nil {
			panic(err)
		}
	}

	write(uint8(1))
	write(uint8(*maxOrder))
	write(uint8(*trainPercent))
	write(uint8(0))

	write(uint64(urls))
	write(uint64(trainURLs))
	write(uint64(len(contexts)))

	for _, context := range contexts {
		write(context.order)
		write(context.key)
		write(context.total)
		write(uint8(len(context.symbols)))

		for _, entry := range context.symbols {
			write(entry.symbol)
			write(entry.count)
		}
	}

	info, err := out.Stat()
	if err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("model written")
	fmt.Printf("output:          %s\n", *output)
	fmt.Printf("contexts:        %d\n", len(contexts))
	fmt.Printf("retained entries: %d\n", len(entries))
	fmt.Printf("model size:      %d bytes\n", info.Size())
}

func dir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}

			return path[:i]
		}
	}

	return "."
}
