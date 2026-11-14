// Command opentitam is a terminal-based AI coding assistant (OpenCode-style,
// local-first). Branding: OpenCode → OpenTitam.
package main

import (
	"os"

	"github.com/mr-daaku/opentitam/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:]))
}
