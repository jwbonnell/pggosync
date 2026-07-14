package opts

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/sync/data"
)

// TableArg holds the parsed result of a --table argument including optional scrub rules.
type TableArg struct {
	Schema     string
	Table      string
	Filter     string
	ScrubRules []config.ScrubRule
}

// ParseGroupArg parses "groupID" or "groupID:p1,p2,…" and returns the group ID and a positional params slice.
// A group with no params returns a nil slice (not [""]), so ApplyParamToFilter leaves any {N}
// placeholders untouched instead of substituting an empty string into the WHERE clause.
func ParseGroupArg(arg string) (string, []string, error) {
	groupID, params, err := parsePrimaryArg(arg)
	if err != nil {
		return "", nil, fmt.Errorf("opts.ParseGroupArg: %w", err)
	}
	if params == "" {
		return groupID, nil, nil
	}
	return groupID, strings.Split(params, ","), nil
}

// ParseTableArg parses "schema.table" or "schema.table:filter" and returns schema, table name, and optional filter.
func ParseTableArg(arg string) (string, string, string, error) {
	fullTableName, filter, err := parsePrimaryArg(arg)
	if err != nil {
		return "", "", "", fmt.Errorf("opts.ParseTableArg: %w", err)
	}

	schema, table, err := ParseFullTableName(fullTableName)
	if err != nil {
		return "", "", "", fmt.Errorf("opts.ParseTableArg: %w", err)
	}

	return schema, table, strings.Trim(filter, "\""), nil
}

// ParseTableArgWithScrub parses "schema.table" or "schema.table:filter" or "schema.table:filter:col1=rule1,col2=rule2"
// and returns a TableArg with schema, table, filter, and optional scrub rules.
func ParseTableArgWithScrub(arg string) (TableArg, error) {
	parts := splitTableArg(arg)
	if len(parts) < 1 {
		return TableArg{}, errors.New("opts.ParseTableArgWithScrub: empty argument")
	}

	schema, table, err := ParseFullTableName(parts[0])
	if err != nil {
		return TableArg{}, fmt.Errorf("opts.ParseTableArgWithScrub: %w", err)
	}

	result := TableArg{Schema: schema, Table: table}

	if len(parts) >= 2 {
		result.Filter = strings.Trim(parts[1], "\"")
	}

	if len(parts) >= 3 {
		rules, err := parseScrubRules(parts[2])
		if err != nil {
			return TableArg{}, fmt.Errorf("opts.ParseTableArgWithScrub: %w", err)
		}
		result.ScrubRules = rules
	}

	return result, nil
}

// splitTableArg splits a table argument on colons, but respects quoted sections for the filter.
// Both double-quoted identifiers and single-quoted SQL string literals are honored, so colons
// inside them (e.g. a time literal '2024-01-01 12:30:00') do not split the filter. Splits at most
// twice (table, filter, scrub) so colons inside scrub rule params survive:
// "schema.table:filter:col=static:foo" → ["schema.table", "filter", "col=static:foo"]
func splitTableArg(arg string) []string {
	var parts []string
	var current strings.Builder
	inDouble := false
	inSingle := false

	for i := 0; i < len(arg); i++ {
		ch := arg[i]
		switch {
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == ':' && !inDouble && !inSingle && len(parts) < 2:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// parseScrubRules parses "col1=rule1,col2=rule2" into []ScrubRule.
func parseScrubRules(scrubStr string) ([]config.ScrubRule, error) {
	var rules []config.ScrubRule
	for _, pair := range strings.Split(scrubStr, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eqIdx := strings.Index(pair, "=")
		if eqIdx < 1 {
			return nil, fmt.Errorf("invalid scrub rule %q (expected column=rule)", pair)
		}
		col := strings.TrimSpace(pair[:eqIdx])
		rule := strings.TrimSpace(pair[eqIdx+1:])
		if col == "" || rule == "" {
			return nil, fmt.Errorf("invalid scrub rule %q (column and rule must not be empty)", pair)
		}
		if !data.IsValidRule(rule) {
			return nil, fmt.Errorf("unsupported scrub rule %q (valid: %s)", rule, strings.Join(data.SupportedRules, ", "))
		}
		rules = append(rules, config.ScrubRule{Column: col, Rule: rule})
	}
	return rules, nil
}

// parsePrimaryArg splits on the first colon to separate the primary identifier from an optional secondary value.
func parsePrimaryArg(arg string) (string, string, error) {
	parts := strings.Split(arg, ":")
	switch {
	case len(parts) < 1 || len(parts) > 2 || parts[0] == "":
		return "", "", errors.New("invalid argument")
	case len(parts) == 1:
		return parts[0], "", nil
	default:
		return parts[0], parts[1], nil
	}
}

// ParseFullTableName returns schema and table from "schema.table"; defaults schema to "public" for bare table names.
func ParseFullTableName(name string) (string, string, error) {
	var schema, table string
	parts := strings.Split(name, ".")
	switch len(parts) {
	case 1:
		schema = "public"
		table = parts[0]
	case 2:
		schema = parts[0]
		table = parts[1]
	default:
		schema = ""
		table = ""
	}

	if schema == "" || table == "" {
		return "", "", errors.New("opts.parseFullTableName: schema or table is empty")
	}

	return schema, table, nil
}
