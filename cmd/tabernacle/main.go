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

	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/songfs"
	"github.com/jalmena/tabernacle/internal/web"
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

	if err := ensureDirs(cfg); err != nil {
		return err
	}

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

	store, err := songfs.Open(cfg.SongsDir, cfg.TrashDir, cfg.MaxSongBytes)
	if err != nil {
		return err
	}
	defer store.Close()

	idx, err := index.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	lib := library.New(store, idx, log)

	// Scan before serving, so the first request sees a warm index rather than an
	// empty library. On a large first run this costs a second or two, which is
	// the right trade for a personal tool.
	if rep, err := lib.SyncIfNeeded(ctx); err != nil {
		return fmt.Errorf("initial scan: %w", err)
	} else if rep.Scanned > 0 {
		log.Info("indexed the collection", "songs", rep.Scanned, "failed", rep.Failed)
	}
	lib.StartPeriodicSync(ctx, cfg.ScanInterval)

	srv2, err := web.New(lib, cfg, log)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: srv2.Handler(),
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

// ensureDirs creates the directories Tabernacle owns.
//
// The songs directory is created rather than required, so that a first run
// against an empty volume works instead of failing on a missing path.
func ensureDirs(cfg config.Config) error {
	for _, dir := range []string{cfg.DataDir, cfg.SongsDir, cfg.TrashDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
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
