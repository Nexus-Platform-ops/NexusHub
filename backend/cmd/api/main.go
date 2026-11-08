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
	shutdownTracing, err := observability.InitTracing(ctx, "nexus-api", cfg.Env, observability.TracingConfigFromEnv())
	if err != nil {
		log.Fatalf("tracing: %v", err)
	}
	defer func() {
		if err := shutdownTracing(context.Background()); err != nil {
			log.Printf("tracing shutdown: %v", err)
		}
	}()

	st, err := store.New(ctx, cfg.SocialDatabaseURL, cfg.DatingDatabaseURL, cfg.RedisAddr,
		store.WithObservability())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	jwtSigner := auth.New(cfg.JWTSecret)
	srv, err := api.New(st, jwtSigner, cfg)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
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

	// Admin/metrics listener: Prometheus scrapes /metrics here, separate
	// from the public surface. Empty METRICS_ADDR disables it.
	if cfg.MetricsAddr != "" {
		metricsServer := &http.Server{
			Addr:              cfg.MetricsAddr,
			Handler:           srv.Metrics.Handler(),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			log.Printf("metrics listening on %s", cfg.MetricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("metrics listen: %v", err)
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = metricsServer.Shutdown(shutdownCtx)
		}()
	}

	// Safety SLA gauge: age of the oldest open HIGH-severity report.
	go srv.RunSafetyGauge(ctx, 30*time.Second)

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
