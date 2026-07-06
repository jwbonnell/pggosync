package data

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// SupportedRules returns the list of predefined scrub rule IDs.
var SupportedRules = []string{
	"hash",
	"redact",
	"null",
	"random_int",
	"random_float",
	"random_email",
	"partial",
	"static",
}

// SQLExpression returns a SQL expression for anonymising column values by ruleID.
// The expression executes on the source database inside the prefetch COPY query,
// so raw values never leave the source. The column argument must be the SQL-ready
// column name (already double-quoted if it is a reserved word).
// For rules that need parameters (partial, static), params are passed via the rule
// string using the format "rule:param", e.g. "partial:3" or "static:test@example.com".
func SQLExpression(ruleID string, column string) string {
	rule, param := splitRuleParam(ruleID)

	switch rule {
	case "hash":
		return fmt.Sprintf("MD5(%s::text)", column)
	case "redact":
		return "'***REDACTED***'"
	case "null":
		return "NULL"
	case "random_int":
		return "(RANDOM() * 100000)::int"
	case "random_float":
		return "(RANDOM() * 1000)::numeric(10,2)"
	case "random_email":
		return "CONCAT('user', FLOOR(RANDOM()*100000)::int, '@example.com')"
	case "partial":
		n := 3
		if param != "" {
			if parsed, err := strconv.Atoi(param); err == nil {
				n = parsed
			}
		}
		if n < 1 {
			n = 1
		}
		return fmt.Sprintf("CASE WHEN LENGTH(%s::text) > %d THEN LEFT(%s::text, %d) || '***' ELSE %s::text END", column, n, column, n, column)
	case "static":
		if param == "" {
			param = "***"
		}
		return fmt.Sprintf("'%s'::text", strings.ReplaceAll(param, "'", "''"))
	default:
		return ""
	}
}

// IsValidRule returns true when ruleID matches one of the predefined scrub rules
// and, for parameterized rules, the param is well-formed.
func IsValidRule(ruleID string) bool {
	rule, param := splitRuleParam(ruleID)
	if !slices.Contains(SupportedRules, rule) {
		return false
	}
	if rule == "partial" && param != "" {
		n, err := strconv.Atoi(param)
		return err == nil && n >= 1
	}
	return true
}

// RuleLabel returns a human-readable label for the rule (e.g. "partial(3)" for "partial:3").
func RuleLabel(ruleID string) string {
	rule, param := splitRuleParam(ruleID)
	if param != "" {
		return fmt.Sprintf("%s(%s)", rule, param)
	}
	return rule
}

// splitRuleParam splits "rule:param" into its parts; returns the rule and param (empty if none).
func splitRuleParam(ruleID string) (rule, param string) {
	idx := strings.Index(ruleID, ":")
	if idx < 0 {
		return ruleID, ""
	}
	return ruleID[:idx], ruleID[idx+1:]
}
