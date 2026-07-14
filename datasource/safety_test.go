package datasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsLocalHost covers the destination safety gate. It runs in short mode because
// IsLocalHost only parses the connection URL and never touches the database.
func TestIsLocalHost(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"localhost with creds", "postgres://u:p@localhost:5445/db", true},
		{"127.0.0.1 with creds", "postgres://u:p@127.0.0.1:5445/db", true},
		{"ipv6 loopback", "postgres://u:p@[::1]:5445/db", true},
		{"localhost without password", "postgres://u@localhost:5445/db", true},
		{"localhost no port", "postgres://u:p@localhost/db", true},
		{"subdomain spoof is rejected", "postgres://u:p@localhost.evil.com/db", false},
		{"prefix spoof is rejected", "postgres://u:p@127.0.0.1.evil.com/db", false},
		{"remote host", "postgres://u:p@db.example.com:5432/db", false},
		{"garbage url", "://not-a-url", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &ReaderDataSource{Url: tc.url}
			assert.Equal(t, tc.want, r.IsLocalHost(context.Background()))
		})
	}
}
