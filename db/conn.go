package db

import (
	"fmt"
	"math/rand"
	"time"
)

func BuildUrl(host string, port int, username string, password string, database string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s", username, password, host, port, database)
}

func GenTempTableName(seed int64, sourceTable string) string {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	r := rand.New(rand.NewSource(seed))
	min := 1
	max := 100000
	return fmt.Sprintf("pggosync_%s_%d", sourceTable, r.Intn(max-min+1)+min)
}
