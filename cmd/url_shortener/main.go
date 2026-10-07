package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/Sujas-Aggarwal/url-compressor/internals/codec"
	"github.com/Sujas-Aggarwal/url-compressor/internals/helper"
)

func main() {
	if len(os.Args) != 3 {
		helper.PrintHelpMessage()
		return
	}

	switch os.Args[1] {
	case "encode":
		encode(os.Args[2])

	case "decode":
		decode(os.Args[2])

	default:
		helper.PrintHelpMessage()
	}
}

func encode(rawURL string) {
	data, bitLen, err := codec.Encode(rawURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode error:", err)
		os.Exit(1)
	}

	// CLI envelope:
	//
	//   [8 bytes bit length][encoded bytes]
	//
	// The codec itself still returns:
	//
	//   []byte + bitLen
	//
	// This envelope only exists so the CLI can transport both pieces
	// of information in a single string.

	output := make([]byte, 8+len(data))

	binary.BigEndian.PutUint64(output[:8], bitLen)
	copy(output[8:], data)

	fmt.Println(hex.EncodeToString(output))
}

func decode(encoded string) {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode error: invalid encoded data:", err)
		os.Exit(1)
	}

	if len(data) < 8 {
		fmt.Fprintln(os.Stderr, "decode error: encoded data is too short")
		os.Exit(1)
	}

	bitLen := binary.BigEndian.Uint64(data[:8])
	payload := data[8:]

	result, err := codec.Decode(payload, bitLen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode error:", err)
		os.Exit(1)
	}

	fmt.Println(result)
}
