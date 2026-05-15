package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCmd builds the `mockta version` subcommand. The build info is
// threaded in from main via NewRootCmd, so the values come from -ldflags
// at build time rather than from package-level mutable state.
func newVersionCmd(build BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the binary version and commit",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(),
				"mockta version %s (commit %s)\n", build.Version, build.Commit)
		},
	}
}
