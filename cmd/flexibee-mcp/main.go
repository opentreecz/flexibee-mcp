package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/opentreecz/flexibee-mcp/internal/config"
	"github.com/opentreecz/flexibee-mcp/internal/mcpserver"
	"github.com/opentreecz/flexibee-mcp/internal/observability"
	"github.com/opentreecz/flexibee-mcp/internal/service"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.BoolVar(showVersion, "v", false, "print version information and exit (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("flexibee-mcp %s (commit: %s, date: %s)\n", version, commit, date)
		return
	}

	logger := observability.DefaultLogger

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	svc, err := service.NewService(cfg)
	if err != nil {
		logger.Error("failed to initialize service", "error", err)
		os.Exit(1)
	}

	srv := mcpserver.New(svc, version)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := srv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
