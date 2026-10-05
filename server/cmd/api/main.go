// Command api runs the Pantheon Spins HTTP server.
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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"pantheon-spins/server/internal/auth"
	"pantheon-spins/server/internal/config"
	"pantheon-spins/server/internal/dbmigrate"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/catalog"
	"pantheon-spins/server/internal/httpapi"
	"pantheon-spins/server/internal/play"
	"pantheon-spins/server/internal/ratelimit"
	"pantheon-spins/server/internal/wallet"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := dbmigrate.Up(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	ropts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("redis url: %w", err)
	}
	rdb := redis.NewClient(ropts)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	registry := games.NewRegistry()
	if err := catalog.Register(registry, cfg.GameConfigDir); err != nil {
		return err
	}

	var mailer auth.Mailer = auth.LogMailer{Log: log}
	if cfg.Mailer == "smtp" {
		mailer = auth.SMTPMailer{Addr: cfg.SMTPAddr, From: cfg.SMTPFrom, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword}
	}

	sessions := auth.NewSessions(rdb, cfg.SessionTTL)
	authSvc, err := auth.NewService(pool, sessions, mailer, log, auth.Options{
		Argon2:      auth.DefaultArgon2,
		SignupBonus: cfg.SignupBonus,
		AppBaseURL:  cfg.AppBaseURL,
	})
	if err != nil {
		return err
	}
	var google *auth.Google
	if cfg.GoogleEnabled() {
		google = auth.NewGoogle(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL, rdb)
	}
	walletSvc := wallet.NewService(pool)

	handler := httpapi.NewHandler(httpapi.Deps{
		Log:     log,
		Auth:    authSvc,
		Google:  google,
		Wallet:  walletSvc,
		Play:    play.NewService(pool, walletSvc, registry),
		Games:   registry,
		Limiter: ratelimit.New(rdb),
		Health: func(ctx context.Context) error {
			return errors.Join(pool.Ping(ctx), rdb.Ping(ctx).Err())
		},
		Settings: httpapi.Settings{
			AppBaseURL:     cfg.AppBaseURL,
			AllowedOrigins: cfg.AllowedOrigins,
			CookieSecure:   cfg.CookieSecure,
			CSRFSecret:     cfg.CSRFSecret,
			TrustProxy:     cfg.TrustProxy,
		},
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "games", len(registry.List()), "google_auth", google != nil, "mailer", cfg.Mailer)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
