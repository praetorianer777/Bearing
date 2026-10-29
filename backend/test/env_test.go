//go:build integration

// Package test is the integration suite: it runs against the compose stack (make test-integration), with the
// addresses mk/go.mk passes in.
package test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/praetorianer777/bearing/backend/internal/config"
)

func env(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("%s is not set; run the suite with make test-integration", key)
	}
	return v
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func loadConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// superuser connects to url as the stack's superuser, for what the suite sets up around the code under test.
func superuser(t *testing.T, key string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, key))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func valkey(t *testing.T) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(env(t, "BEARING_VALKEY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// valkeyFor is a client that lives as long as the suite, for what the suite shares between its tests.
func valkeyFor(t *testing.T, cfg config.Config) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(cfg.Valkey.URL)
	if err != nil {
		t.Fatal(err)
	}
	return redis.NewClient(opts)
}
