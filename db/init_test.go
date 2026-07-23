package db

import (
	"strings"
	"testing"
)

func TestInitSQLDoesNotSeedDefaultAdminCredentials(t *testing.T) {
	sql := string(InitSQL())
	for _, forbidden := range []string{
		"Seed default admin user",
		"admin@example.com",
		"password 'admin'",
		"INSERT INTO users (id, sub, email, provider, password_hash",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("init.sql still contains default admin credential seed marker %q", forbidden)
		}
	}
	if !strings.Contains(sql, "dispatcher-internal") || !strings.Contains(sql, "nopsai-admin") {
		t.Fatalf("init.sql lost required role bootstrap statements")
	}
}
