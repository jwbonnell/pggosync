package db

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

type Trigger struct {
	Name      string `db:"name"`
	Internal  bool   `db:"internal"`
	Enabled   bool   `db:"enabled"`
	Integrity bool   `db:"integrity"`
}

func DisableUserTriggers(ctx context.Context, db *pgx.Conn, tables []Table) error {
	var err error
	for _, t := range tables {
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DISABLE TRIGGER USER", t.FullName()))
		if err != nil {
			return fmt.Errorf("pgx.Exec: disable user triggers: %w", err)
		}
	}

	return nil
}

func RestoreUserTriggers(ctx context.Context, db *pgx.Conn, tables []Table) error {
	var err error
	for _, t := range tables {
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ENABLE TRIGGER USER", t.FullName()))
		if err != nil {
			return fmt.Errorf("pgx.Exec: disable user triggers: %w", err)
		}
	}

	return nil
}
