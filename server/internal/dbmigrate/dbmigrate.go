// Package dbmigrate applies the embedded SQL migrations with golang-migrate.
package dbmigrate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	dbschema "pantheon-spins/server/db"
)

// Up applies all pending migrations. databaseURL is a postgres:// URL.
func Up(databaseURL string) error {
	src, err := iofs.New(dbschema.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toPgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}

func toPgx5URL(u string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(u, p); ok {
			return "pgx5://" + rest
		}
	}
	return u
}
