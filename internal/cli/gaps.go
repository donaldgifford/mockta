package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/mockta/internal/gaps"
)

// newGapsCmd assembles the `mockta gaps` command tree. The list and
// export subcommands operate on the static registry; the runtime
// flag on `list` is reserved for the audit-log integration (Phase 5
// stretch — not currently wired since the in-process store goes away
// when the binary exits and there is no persistent log to query yet).
func newGapsCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "gaps",
		Short: "Inspect or export the mockta gap registry",
		Long: "The gap registry lists every Okta API surface that mockta " +
			"does not implement. Use `gaps list` to print the table " +
			"and `gaps export` to emit the canonical markdown that " +
			"ships as docs/gaps.md.",
	}
	root.AddCommand(newGapsListCmd())
	root.AddCommand(newGapsExportCmd())
	return root
}

func newGapsListCmd() *cobra.Command {
	var runtime bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print the gap registry in tabular form",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runtime {
				return fmt.Errorf(
					"--runtime is reserved for the audit-log " +
						"integration; see IMPL-0001 Phase 5",
				)
			}
			return writeTabular(cmd.OutOrStdout(), gaps.Static().All())
		},
	}
	cmd.Flags().BoolVar(&runtime, "runtime", false,
		"reserved — query the running mockta's audit log (not yet implemented)")
	return cmd
}

func newGapsExportCmd() *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Emit docs/gaps.md from the static registry",
		Long: "Writes the markdown rendering of the static registry. " +
			"The output is deterministic across runs; the CI drift " +
			"check pipes this command through `diff` against the " +
			"committed file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if outPath == "" || outPath == "-" {
				return gaps.Markdown(cmd.OutOrStdout(), gaps.Static().All())
			}
			return writeExport(outPath)
		},
	}
	cmd.Flags().StringVarP(&outPath, "out", "o", "",
		"output path; empty or '-' writes to stdout")
	return cmd
}

// writeExport renders the markdown registry to outPath. Split out of
// the cobra RunE so the close-error handling can use a named return
// without inflating the command body.
func writeExport(outPath string) (err error) {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", outPath, cerr)
		}
	}()
	return gaps.Markdown(f, gaps.Static().All())
}

// writeTabular prints the registry using text/tabwriter so the
// terminal output stays aligned. Sorted by ID for stable ordering.
func writeTabular(w io.Writer, entries []gaps.Gap) error {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tENDPOINT\tRESOURCE\tSTATUS"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, g := range entries {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			g.ID, g.Endpoint, g.Resource, g.Status); err != nil {
			return fmt.Errorf("write row %s: %w", g.ID, err)
		}
	}
	return tw.Flush()
}
