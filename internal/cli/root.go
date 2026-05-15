// Package cli wires the cobra command tree for the mockta binary.
//
// The package exposes a single constructor, NewRootCmd, so the build-info
// values defined in main can be threaded through to subcommands without
// package-level mutable state.
package cli

import (
	"github.com/spf13/cobra"
)

// BuildInfo carries the values injected via -ldflags into the main package.
// It is passed into NewRootCmd so subcommands like `version` can render them
// without reaching into globals.
type BuildInfo struct {
	Version string
	Commit  string
}

// NewRootCmd constructs the root cobra command and registers every
// subcommand. The returned command is ready to Execute.
func NewRootCmd(build BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:   "mockta",
		Short: "Lightweight Okta mock for Terraform and Go service tests",
		Long: "mockta is a lightweight, embeddable Okta mock for Terraform " +
			"acceptance tests and Go service tests. It speaks the slice of " +
			"the Okta Management API our Terraform modules and services " +
			"actually exercise.",
		// Silence usage on RunE error — error context is the actionable
		// signal, not the usage block.
		SilenceUsage: true,
	}

	root.AddCommand(newServeCmd())
	root.AddCommand(newHealthcheckCmd())
	root.AddCommand(newVersionCmd(build))
	root.AddCommand(newGapsCmd())

	return root
}
