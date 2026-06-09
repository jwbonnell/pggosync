package data

// Scrub returns a SQL expression for anonymising a column value by ruleID. Currently incomplete — only random_int is handled.
func Scrub(ruleID string, tableName string, column string, primaryKey int) string {
	//TODO
	switch ruleID {
	case "random_int":
		return "(RANDOM() * 100)::int"
	default:
		return ""
	}
}
