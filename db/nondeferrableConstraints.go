package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type NonDeferrableConstraints struct {
	Schema         string `db:"schema"`
	Table          string `db:"table"`
	ConstraintName string `db:"constraint_name"`
}

func DeferConstraints(ctx context.Context, db *pgx.Conn, ndc []NonDeferrableConstraints) error {
	var err error
	for _, n := range ndc {
		t := Table{Schema: n.Schema, Name: n.Table}
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ALTER CONSTRAINT %s DEFERRABLE", t.FullName(), n.ConstraintName))
		if err != nil {
			return fmt.Errorf("pgx.Exec: set non-deferrable constraint to deferrable: %w", err)
		}
	}

	_, err = db.Exec(ctx, "SET CONSTRAINTS ALL DEFERRED")
	if err != nil {
		return fmt.Errorf("pgx.Exec:SET ALL CONTRAINTS DEFERRED: %w", err)
	}

	return nil
}

func RestoreContraints(ctx context.Context, db *pgx.Conn, ndc []NonDeferrableConstraints) error {
	_, err := db.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE")
	if err != nil {
		return fmt.Errorf("pgx.Exec:SET ALL CONTRAINTS IMMEDIATE: %w", err)
	}

	for _, n := range ndc {
		t := Table{Schema: n.Schema, Name: n.Table}
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ALTER CONSTRAINT %s NOT DEFERRABLE", t.FullName(), n.ConstraintName))
		if err != nil {
			return fmt.Errorf("pgx.Exec: set non-deferrable constraint to NOT deferrable: %w", err)
		}
	}
	return nil
}
