package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RedLicorice/GoGitIt/internal/api"
	"github.com/RedLicorice/GoGitIt/internal/auth"
	"github.com/RedLicorice/GoGitIt/internal/config"
	"github.com/RedLicorice/GoGitIt/internal/repo"
	"github.com/RedLicorice/GoGitIt/internal/settings"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	slog.Info("starting gogitit",
		"addr", cfg.Server.Addr,
		"auth_enabled", cfg.Auth.Enabled,
		"repos_dir", cfg.Storage.ReposDir,
	)

	// Repository registry: persistent index of repos managed by the app.
	registry, err := repo.NewRegistry(cfg.Storage.StateDir, cfg.Storage.ReposDir)
	if err != nil {
		slog.Error("failed to initialize repo registry", "err", err)
		os.Exit(1)
	}

	// User settings: identity + credentials, JSON-persisted in the state dir.
	settingsStore, err := settings.NewStore(cfg.Storage.StateDir)
	if err != nil {
		slog.Error("failed to initialize settings store", "err", err)
		os.Exit(1)
	}

	// Auth provider: real OIDC when enabled, passthrough otherwise.
	authProvider, err := auth.New(context.Background(), cfg.Auth)
	if err != nil {
		slog.Error("failed to initialize auth", "err", err)
		os.Exit(1)
	}

	router := api.NewRouter(cfg, authProvider, registry, settingsStore)

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	go func() {
		slog.Info("http server listening", "addr", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}
