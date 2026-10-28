// Command sammy is Sammy's single binary. Subcommands:
//
//	sammy serve    run the HTTP API
//	sammy worker   run River background workers
//	sammy migrate  apply goose + River migrations
//	sammy eval     run the eval harness (not implemented yet)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kerliula/sammy-memory/internal/config"
	"github.com/Kerliula/sammy-memory/internal/db"
	"github.com/Kerliula/sammy-memory/internal/jobs"
	"github.com/Kerliula/sammy-memory/internal/server"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sammy <serve|worker|migrate|eval>")
		return 2
	}
	var err error
	switch args[0] {
	case "serve":
		err = runServe()
	case "worker":
		err = runWorker()
	case "migrate":
		err = runMigrate()
	case "eval":
		fmt.Fprintln(os.Stderr, "eval: not implemented")
		return 1
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\nusage: sammy <serve|worker|migrate|eval>\n", args[0])
		return 2
	}
	if err != nil {
		slog.Error("command failed", "command", args[0], "error", err)
		return 1
	}
	return 0
}

func runServe() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := cfg.Logger()
	slog.SetDefault(logger)

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.New(pool),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("serving", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func runWorker() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := cfg.Logger()
	slog.SetDefault(logger)

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	client, err := jobs.NewWorkerClient(pool)
	if err != nil {
		return err
	}
	if err := client.Start(context.Background()); err != nil {
		return fmt.Errorf("start river client: %w", err)
	}
	logger.Info("worker started")

	<-ctx.Done()
	logger.Info("shutting down")
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		return fmt.Errorf("stop river client: %w", err)
	}
	return nil
}

func runMigrate() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := cfg.Logger()
	slog.SetDefault(logger)

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	logger.Info("migrations applied")
	return nil
}
