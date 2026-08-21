// Package app assembles a running Tabernacle out of its parts.
//
// It exists so that there is one description of how the pieces fit together
// rather than two. The command wires nothing itself, and neither do the
// end-to-end tests, which is what makes those tests worth having: they drive the
// same stack the binary serves instead of a lookalike free to drift from it.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jalmena/tabernacle/internal/account"
	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/songfs"
	"github.com/jalmena/tabernacle/internal/web"
)

// App is an assembled instance: a handler to serve and the resources behind it.
//
// Library is exported because the collection is the product and callers have
// legitimate business with it — a scan after changing files on disk, most of
// all. The store and the index are not, because everything they offer should be
// reached through the library.
type App struct {
	Handler  http.Handler
	Library  *library.Library
	Accounts *account.Store

	store *songfs.Store
	index *index.Index
}

// Build opens the collection, the index and the HTTP handler.
//
// The caller owns the result and must Close it. The first scan happens here
// rather than being left to the caller: serving before it would answer from an
// index that does not yet describe the directory, and startup is exactly when
// the directory may have changed with nobody watching.
//
// Periodic scanning is deliberately not started here. It belongs to a long-lived
// process, and a test that wants a scan should ask for one at a moment it chose.
func Build(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	if err := EnsureDirs(cfg); err != nil {
		return nil, err
	}

	store, err := songfs.Open(cfg.SongsDir, cfg.TrashDir, cfg.MaxSongBytes)
	if err != nil {
		return nil, err
	}

	idx, err := index.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}

	cost := cfg.PasswordCost
	if cost <= 0 {
		cost = account.DefaultCost
	}
	acc, err := account.Open(cfg.DataDir, cfg.AccountsFile, cost)
	if err != nil {
		return nil, errors.Join(err, idx.Close(), store.Close())
	}

	lib := library.New(store, idx, log)
	a := &App{Library: lib, Accounts: acc, store: store, index: idx}

	// The bootstrap administrator is created, or has their password reset, from
	// the environment. Resetting is the point: it is the documented way back in
	// after a forgotten password, and it asks for shell access to the host, which
	// is the right bar for it.
	if cfg.AdminUser != "" {
		created, err := acc.Bootstrap(cfg.AdminUser, cfg.AdminPassword.Reveal())
		if err != nil {
			return nil, errors.Join(fmt.Errorf("bootstrap administrator: %w", err), a.Close())
		}
		if created {
			log.Info("created the administrator named in the environment", "user", cfg.AdminUser)
		} else {
			log.Warn("reset the administrator's password from the environment", "user", cfg.AdminUser)
		}
	}

	if rep, err := lib.Sync(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("initial scan: %w", err), a.Close())
	} else if rep.Scanned > 0 {
		log.Info("indexed the collection", "songs", rep.Scanned, "failed", rep.Failed)
	}

	srv, err := web.New(lib, acc, cfg, log)
	if err != nil {
		return nil, errors.Join(err, a.Close())
	}
	a.Handler = srv.Handler()
	return a, nil
}

// Close releases the index, the accounts and the collection.
func (a *App) Close() error {
	// The index first: it is the derived half, and closing it cannot invalidate
	// anything the store still needs.
	return errors.Join(a.index.Close(), a.Accounts.Close(), a.store.Close())
}

// EnsureDirs creates the directories Tabernacle owns.
//
// The songs directory is created rather than required, so that a first run
// against an empty volume works instead of failing on a missing path.
func EnsureDirs(cfg config.Config) error {
	for _, dir := range []string{cfg.DataDir, cfg.SongsDir, cfg.TrashDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}
