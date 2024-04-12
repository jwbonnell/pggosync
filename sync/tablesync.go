package sync

import (
	"bytes"
	"context"
	"fmt"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

type TableSync struct {
	source      *datasource.ReaderDataSource
	destination *datasource.ReadWriteDatasource
}

func NewTableSync(source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource) *TableSync {
	return &TableSync{
		source:      source,
		destination: dest,
	}
}

func (t *TableSync) Sync(ctx context.Context, table string, filter string, truncate bool, preserve bool) error {
	if !truncate {
		//TODO PK CHECK
		ttName := db.GenTempTableName(0)
		err := t.destination.CreateTempTable(ctx, ttName, table)
		if err != nil {
			return fmt.Errorf("datasource.CreateTempTable: %w", err)
		}

		err = t.copyTo(ctx, table, ttName, "")
		if err != nil {
			return fmt.Errorf("TableSync.copyTo temp table %s: %w", ttName, err)
		}

		onConflict := "NOTHING"
		if !preserve {
			onConflict = 
		}

		err = t.destination.InsertFromTempTable(ctx, ttName, table, []string{}, onConflict, action)

	} else {

	}

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

func (t *TableSync) copyTo(ctx context.Context, sourceTable string, destTable string, filter string) error {
	//TODO aconsider merging copy anc copyTo, keeping separate for now
	var buf bytes.Buffer
	sconn := t.source.DB.PgConn()
	_, err := sconn.CopyTo(ctx, &buf, fmt.Sprintf("COPY (SELECT * FROM %s %s ) TO STDOUT", sourceTable, filter))
	if err != nil {
		return err
	}

	dconn := t.destination.DB.PgConn()
	_, err = dconn.CopyFrom(ctx, &buf, fmt.Sprintf("COPY %s FROM STDIN", destTable))
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
