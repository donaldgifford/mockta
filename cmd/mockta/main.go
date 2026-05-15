// Package main is the entry point for the mockta binary.
//
// Build-info variables (version, commit) are injected via -ldflags by
// the justfile's build recipes.
package main

import (
	"os"

	"github.com/donaldgifford/mockta/internal/cli"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	root := cli.NewRootCmd(cli.BuildInfo{Version: version, Commit: commit})
	if err := root.Execute(); err != nil {
		// cobra has already printed the error; just exit non-zero.
		os.Exit(1)
	}
}
