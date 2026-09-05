// Command tocli is a process-per-torrent terminal torrent client.
package main

import (
	"fmt"
	"os"

	"github.com/pratts/tocli/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tocli:", err)
		os.Exit(1)
	}
}
