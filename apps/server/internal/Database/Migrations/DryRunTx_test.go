package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func TestApplyStatementsNoticesAnEndedTransaction(t *testing.T) {
	url := config.TestDatabaseURL()
	if url == "" {
		if config.RunningInCI() {
			t.Fatal(config.EnvTestDatabaseURL + " must be set in CI")
		}
		t.Skip(config.EnvTestDatabaseURL + " not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	failed, err := applyStatements(ctx, tx, []string{"SELECT 1", "COMMIT", "SELECT 2"})
	if !errors.Is(err, ErrTransactionEnded) {
		t.Fatalf("got %v, want ErrTransactionEnded", err)
	}
	if failed != 1 {
		t.Errorf("failed at statement %d, want 1", failed)
	}
}
