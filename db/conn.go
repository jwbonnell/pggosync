package db

import (
	"fmt"
	"time"
)

// BuildUrl constructs a postgres:// connection URL from individual parameters.
func BuildUrl(host string, port int, username string, password string, database string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s", username, password, host, port, database)
}

// GenTempTableName produces a unique temp table name; seed=0 uses the current nanosecond timestamp.
func GenTempTableName(seed int64, sourceTable string) string {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return fmt.Sprintf("pggosync_%s_%d", sourceTable, seed)
}
