package cli

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// healthcheckURL is the local probe endpoint. Declared as a var (not
// const) so tests can override it; production callers don't mutate
// it. The default is hardcoded because this command's whole purpose
// is to be invoked by Docker's HEALTHCHECK instruction (and
// terraform's docker_container.healthcheck block), both of which
// always run inside the container against localhost.
var healthcheckURL = "http://localhost:9090/health"

// healthcheckTimeout caps the probe at a duration short enough not to block
// Docker's healthcheck interval, but long enough to absorb a slow startup.
const healthcheckTimeout = 5 * time.Second

// newHealthcheckCmd builds the `mockta healthcheck` subcommand. Exits 0
// when /health returns 200, non-zero otherwise. Wired into Dockerfile
// HEALTHCHECK and terraform's docker_container.healthcheck block.
func newHealthcheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "healthcheck",
		Short: "Probe the local /health endpoint and exit 0 on 200",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), healthcheckTimeout)
			defer cancel()
			return probeHealth(ctx, healthcheckURL)
		},
	}
}

// probeHealth issues a GET against url and returns an error if the
// request fails or the response is not 200. Factored out of the
// cobra RunE so tests can exercise it against an httptest.Server
// without binding the real listener.
func probeHealth(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, url, http.NoBody,
	)
	if err != nil {
		return fmt.Errorf("build healthcheck request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	return nil
}
