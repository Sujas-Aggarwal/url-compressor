package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Sujas-Aggarwal/url-compressor/internals/codec"
)

func main() {
	input := flag.String(
		"input",
		"corpus/training/urls.txt",
		"training URL corpus",
	)

	maxURLs := flag.Int(
		"max-urls",
		0,
		"maximum URLs to process; 0 means all",
	)

	flag.Parse()

	file, err := os.Open(*input)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	model := codec.NewPPMModel()

	scanner := bufio.NewScanner(file)

	// URLs can be up to 4096 bytes in our corpus.
	scanner.Buffer(
		make([]byte, 64*1024),
		16*1024*1024,
	)

	var urls uint64
	var bytes uint64

	start := time.Now()

	for scanner.Scan() {
		line := scanner.Bytes()

		if len(line) == 0 {
			continue
		}

		model.Update(line)

		urls++
		bytes += uint64(len(line))

		if urls%100_000 == 0 {
			fmt.Printf(
				"processed %d URLs | %d bytes | elapsed=%s\n",
				urls,
				bytes,
				time.Since(start).Round(time.Second),
			)
		}

		if *maxURLs > 0 && int(urls) >= *maxURLs {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("PPM MODEL")
	fmt.Println("========================================")

	fmt.Printf("URLs:   %d\n", urls)
	fmt.Printf("Bytes:  %d\n", bytes)
	fmt.Printf(
		"Time:   %s\n",
		time.Since(start).Round(time.Second),
	)

	fmt.Println()
	fmt.Println("CONTEXT COUNTS")
	fmt.Println("----------------------------------------")

	for order := 0; order <= codec.PPMMaxOrder; order++ {
		fmt.Printf(
			"Order %d: %d contexts\n",
			order,
			model.ContextCount(order),
		)
	}
}
