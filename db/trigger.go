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
	RelID     string `db:"tgrelid"`
}

func DisableUserTriggers(ctx context.Context, db *pgx.Conn, triggers []Trigger) error {
	var err error
	for _, t := range triggers {
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DISABLE TRIGGER %s", t.RelID, t.Name))
		if err != nil {
			return fmt.Errorf("pgx.Exec: disable user triggers: %w", err)
		}
	}

	return nil
}

func RestoreUserTriggers(ctx context.Context, db *pgx.Conn, triggers []Trigger) error {
	var err error
	for _, t := range triggers {
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ENABLE TRIGGER %s", t.RelID, t.Name))
		if err != nil {
			return fmt.Errorf("pgx.Exec: disable user triggers: %w", err)
		}
	}

	return nil
}
