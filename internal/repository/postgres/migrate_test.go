package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"quoteservice/internal/repository/postgres"
)

func TestMigrateConcurrent(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{URL: url, ConnectTimeout: 3 * time.Second, MaxConns: 8})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	dir := filepath.Join("..", "..", "..", "..", "migrations")
	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			errs[i] = postgres.Migrate(ctx, pool, dir)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("migrate %d: %v", i, err)
		}
	}
}
