// Command tocli is a process-per-torrent terminal torrent client.
package main

import (
	"fmt"
	"os"

	"github.com/pratts/tocli/internal/cli"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z" (see
// .github/workflows/release.yml, which sets it to the pushed tag); it
// stays "dev" for local `go build`/`go run` and `go install` builds.
var version = "dev"

func main() {
	if err := cli.NewRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tocli:", err)
		os.Exit(1)
	}
}
