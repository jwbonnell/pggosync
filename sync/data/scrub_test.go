package data

import (
	"testing"
)

func TestSQLExpression(t *testing.T) {
	tests := []struct {
		rule   string
		column string
		want   string
	}{
		{"hash", "email", "MD5(email::text)"},
		{"hash", `"order"`, "MD5(\"order\"::text)"},
		{"redact", "ssn", "'***REDACTED***'"},
		{"null", "name", "NULL"},
		{"random_int", "age", "(RANDOM() * 100000)::int"},
		{"random_float", "price", "(RANDOM() * 1000)::numeric(10,2)"},
		{"random_email", "email", "CONCAT('user', FLOOR(RANDOM()*100000)::int, '@example.com')"},
		{"partial", "name", "CASE WHEN LENGTH(name::text) > 3 THEN LEFT(name::text, 3) || '***' ELSE name::text END"},
		{"partial:1", "name", "CASE WHEN LENGTH(name::text) > 1 THEN LEFT(name::text, 1) || '***' ELSE name::text END"},
		{"partial:5", "name", "CASE WHEN LENGTH(name::text) > 5 THEN LEFT(name::text, 5) || '***' ELSE name::text END"},
		{"static", "name", "'***'::text"},
		{"static:test@example.com", "email", "'test@example.com'::text"},
		{"static:O'Brien", "name", "'O''Brien'::text"},
		{"partial:abc", "name", "CASE WHEN LENGTH(name::text) > 3 THEN LEFT(name::text, 3) || '***' ELSE name::text END"},
		{"unknown", "name", ""},
	}

	for _, tt := range tests {
		t.Run(tt.rule+"_"+tt.column, func(t *testing.T) {
			got := SQLExpression(tt.rule, tt.column)
			if got != tt.want {
				t.Errorf("SQLExpression(%q, %q) = %q, want %q", tt.rule, tt.column, got, tt.want)
			}
		})
	}
}

func TestIsValidRule(t *testing.T) {
	valid := []string{"hash", "redact", "null", "random_int", "random_float", "random_email", "partial", "static", "partial:3", "static:foo"}
	for _, r := range valid {
		if !IsValidRule(r) {
			t.Errorf("IsValidRule(%q) = false, want true", r)
		}
	}

	invalid := []string{"unknown", "HASH", "Redact", "", "partial:abc", "partial:0", "partial:-1"}
	for _, r := range invalid {
		if IsValidRule(r) {
			t.Errorf("IsValidRule(%q) = true, want false", r)
		}
	}
}

func TestRuleLabel(t *testing.T) {
	tests := []struct {
		rule string
		want string
	}{
		{"hash", "hash"},
		{"partial", "partial"},
		{"partial:3", "partial(3)"},
		{"static:foo", "static(foo)"},
		{"random_int", "random_int"},
	}

	for _, tt := range tests {
		got := RuleLabel(tt.rule)
		if got != tt.want {
			t.Errorf("RuleLabel(%q) = %q, want %q", tt.rule, got, tt.want)
		}
	}
}

func TestSupportedRulesNotEmpty(t *testing.T) {
	if len(SupportedRules) == 0 {
		t.Error("SupportedRules should not be empty")
	}
	for _, r := range SupportedRules {
		if !IsValidRule(r) {
			t.Errorf("SupportedRules contains %q which is not valid", r)
		}
	}
}
