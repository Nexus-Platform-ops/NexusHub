// Command api runs the Nexus REST API. Configuration comes exclusively from
// the environment (see .env.example); startup fails fast on missing secrets.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nexus/backend/internal/api"
	"nexus/backend/internal/auth"
	"nexus/backend/internal/config"
	"nexus/backend/internal/observability"
	"nexus/backend/internal/store"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("unsafe configuration for APP_ENV=%s: %v", cfg.Env, err)
	}
	flushSentry, err := observability.InitSentry(cfg)
	if err != nil {
		log.Fatalf("sentry: %v", err)
	}
	defer flushSentry()
	if cfg.SentryDSN != "" {
		log.Printf("sentry reporting enabled (environment=%s)", cfg.SentryEnvironment)
	}
	ctx := context.Background()
	st, err := store.New(ctx, cfg.SocialDatabaseURL, cfg.DatingDatabaseURL, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	jwtSigner := auth.New(cfg.JWTSecret)
	srv := api.New(st, jwtSigner, cfg)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("nexus api listening on %s", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
