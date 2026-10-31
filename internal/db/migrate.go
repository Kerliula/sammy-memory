package db

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	sammydb "github.com/Kerliula/sammy-memory/db"
)

// Migrate applies Sammy's goose migrations, then River's own migrations.
// Both are idempotent, so running `sammy migrate` twice is safe.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := migrateGoose(ctx, pool); err != nil {
		return err
	}
	return migrateRiver(ctx, pool)
}

func migrateGoose(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }() // leaves the pgx pool open
	migrationsFS, err := fs.Sub(sammydb.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("sub migrations fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrationsFS)
	if err != nil {
		return fmt.Errorf("create goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("run goose migrations: %w", err)
	}
	return nil
}

func migrateRiver(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("run river migrations: %w", err)
	}
	return nil
}
