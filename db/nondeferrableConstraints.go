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

// DeferConstraints alters each constraint to DEFERRABLE then issues SET CONSTRAINTS ALL DEFERRED for the transaction.
func DeferConstraints(ctx context.Context, db *pgx.Conn, ndc []NonDeferrableConstraints) error {
	var err error
	for _, n := range ndc {
		t := Table{Schema: n.Schema, Name: n.Table}
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ALTER CONSTRAINT %s DEFERRABLE", t.SQLName(), QuoteIdentifier(n.ConstraintName)))
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

// RestoreContraints sets constraints IMMEDIATE then marks each altered constraint NOT DEFERRABLE, reversing DeferConstraints.
func RestoreContraints(ctx context.Context, db *pgx.Conn, ndc []NonDeferrableConstraints) error {
	_, err := db.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE")
	if err != nil {
		return fmt.Errorf("pgx.Exec:SET ALL CONTRAINTS IMMEDIATE: %w", err)
	}

	for _, n := range ndc {
		t := Table{Schema: n.Schema, Name: n.Table}
		_, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ALTER CONSTRAINT %s NOT DEFERRABLE", t.SQLName(), QuoteIdentifier(n.ConstraintName)))
		if err != nil {
			return fmt.Errorf("pgx.Exec: set non-deferrable constraint to NOT deferrable: %w", err)
		}
	}
	return nil
}
