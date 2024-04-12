package db

type Trigger struct {
	Name      string `db:"name"`
	Internal  bool   `db:"internal"`
	Enabled   bool   `db:"enabled"`
	Integrity bool   `db:"integrity"`
}
