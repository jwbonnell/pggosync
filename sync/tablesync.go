package sync

import (
	"bytes"
	"context"
	"fmt"
)

type TableSync struct {
	source      *DataSource
	destination *DataSource
}

func NewTableSync(source *DataSource, dest *DataSource) *TableSync {
	return &TableSync{
		source:      source,
		destination: dest,
	}
}

func (t *TableSync) Sync(ctx context.Context, table string, filter string) error {
	return t.copy(ctx, table, filter)
}

func (t *TableSync) copy(ctx context.Context, table string, filter string) error {
	//TODO add support for only copying certain columns
	var buf bytes.Buffer
	sconn := t.source.DB.PgConn()
	_, err := sconn.CopyTo(ctx, &buf, fmt.Sprintf("COPY (SELECT * FROM %s %s ) TO STDOUT", table, filter))
	if err != nil {
		return err
	}

	dconn := t.destination.DB.PgConn()
	_, err = dconn.CopyFrom(ctx, &buf, fmt.Sprintf("COPY %s FROM STDIN", table))
	if err != nil {
		return err
	}

	return nil
}

func (t *TableSync) handleNonDeferrableConstraints() error {
	//constraints, err := t.source.GetNonDeferrableConstraints()
	/* if err != nil {
		return err
	} */

	//for _, con := range constraints {
	//destination.execute("ALTER TABLE #{quote_ident_full(table)} ALTER CONSTRAINT #{quote_ident(constraint)} DEFERRABLE")
	//}

	//destination.execute("SET CONSTRAINTS ALL DEFERRED")

	// create a transaction on the source
	// to ensure we get a consistent snapshot
	/* source.transaction do
	yield
	end */
	//YIELD here in ruby must return control to another process to do the sync

	// set them back
	// there are 3 modes: DEFERRABLE INITIALLY DEFERRED, DEFERRABLE INITIALLY IMMEDIATE, and NOT DEFERRABLE
	// we only update NOT DEFERRABLE
	// https://www.postgresql.org/docs/current/sql-set-constraints.html

	/* destination.execute("SET CONSTRAINTS ALL IMMEDIATE")

	table_constraints.each do |table, constraints|
	  constraints.each do |constraint|
		 destination.execute("ALTER TABLE #{quote_ident_full(table)} ALTER CONSTRAINT #{quote_ident(constraint)} NOT DEFERRABLE")
	  end
	end */
	return nil
}
