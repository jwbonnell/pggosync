package db

import "fmt"

func BuildUrl(host string, port int, username string, password string, database string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s", username, password, host, port, database)
}
