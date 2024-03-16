func handleNonDeferrableConstraints() error {
	constraints, err := db.GetNonDeferrableConstraints()
	if err != nil {
		return err
	}

	for _, con := range constraints {
		//destination.execute("ALTER TABLE #{quote_ident_full(table)} ALTER CONSTRAINT #{quote_ident(constraint)} DEFERRABLE")
	}

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

}