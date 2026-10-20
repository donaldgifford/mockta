package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/mockta/internal/config"
	"github.com/donaldgifford/mockta/pkg/mockta"
)

// newServeCmd builds the `mockta serve` subcommand, the runtime
// entrypoint. It blocks until SIGINT/SIGTERM, then drains both
// listeners via Server.Start's graceful-shutdown path. The container
// image sets `CMD ["serve"]` so a bare `docker run` lands here.
func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the mockta HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			logger := newLogger(os.Stderr, cfg.LogLevel)
			slog.SetDefault(logger)

			ctx, stop := signal.NotifyContext(
				cmd.Context(), os.Interrupt, syscall.SIGTERM,
			)
			defer stop()

			srv := mockta.New(cfg, logger)
			if err := srv.Start(ctx); err != nil {
				return fmt.Errorf("start server: %w", err)
			}
			return nil
		},
	}
}
