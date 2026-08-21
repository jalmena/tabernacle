// Command tabernacle serves a personal guitar songbook from a directory of
// .song files.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/jalmena/tabernacle/internal/app"
	"github.com/jalmena/tabernacle/internal/config"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3" and
// otherwise comes from the module's build info, so a `go install` build still
// reports something useful.
var version = ""

func main() {
	if err := run(); err != nil {
		// Configuration problems are the user's to fix, so they go to stderr in
		// plain text rather than as a structured log line nobody asked for.
		fmt.Fprintf(os.Stderr, "tabernacle: %v\n", err)
		os.Exit(2)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildVersion())
		return nil
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	// Stop on the signals a container runtime actually sends.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("starting tabernacle",
		"version", buildVersion(),
		"addr", cfg.Addr,
		"songs_dir", cfg.SongsDir,
		"db_path", cfg.DBPath,
		"scan_interval", cfg.ScanInterval,
		"read_only", cfg.ReadOnly,
		"notation", cfg.Notation,
	)
	if cfg.ReadOnly {
		log.Warn("read-only mode: every change to the collection will be refused")
	}

	a, err := app.Build(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()
	a.Library.StartPeriodicSync(ctx, cfg.ScanInterval)

	if n := a.Accounts.Count(); n == 0 {
		log.Warn("no accounts yet: anyone who can reach this instance can change the collection. " +
			"Open Ajustes to claim it")
	} else {
		log.Info("accounts", "count", n)
	}

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: a.Handler,
		// Without this a single idle connection can hold a request open
		// indefinitely; it is the one timeout that is always worth setting.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("listen on %s: %w", cfg.Addr, err)
			return
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	// Give in-flight requests a moment; a print job or a large export should not
	// be cut off by a restart.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("stopped")
	return nil
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}
