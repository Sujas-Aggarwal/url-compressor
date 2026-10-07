package main

import (
	"fmt"
	"os"

	"github.com/Sujas-Aggarwal/url-compressor/internals/codec"
	"github.com/Sujas-Aggarwal/url-compressor/internals/helper"
)

func main() {
	if len(os.Args) != 3 {
		helper.PrintHelpMessage()
	}
	switch os.Args[1] {
	case "encode":
		fmt.Println(codec.Encode(os.Args[2]))
	case "decode":
		fmt.Println(codec.Decode(os.Args[2]))
	default:
		helper.PrintHelpMessage()
	}
}
