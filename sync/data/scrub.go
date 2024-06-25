package data

func Scrub(ruleID string, tableName string, column string, primaryKey int) string {
	//TODO
	switch ruleID {
	case "random_int":
		return "(RANDOM() * 100)::int"
	default:
		return ""
	}
}
