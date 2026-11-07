// Command migrate applies pending SQL migrations and exits. In production it
// runs as a one-shot service before the API starts, so schema changes are
// never applied implicitly by a long-running process.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nexus/backend/internal/store"
)

func main() {
	url := os.Getenv("MIGRATIONS_URL")
	if url == "" {
		log.Fatal("MIGRATIONS_URL is required (superuser connection for schema changes)")
	}
	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "/app/migrations"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	pool, err := store.OpenPool(ctx, url)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("migrate: ping: %v", err)
	}
	if err := store.Migrate(ctx, pool, dir); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("migrations applied from %s", dir)
}
