package main

import (
	"fmt"
	"os"

	"zeroclaw/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zeroclaw:", err)
		os.Exit(1)
	}
}
