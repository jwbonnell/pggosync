package opts

import (
	"errors"
	"fmt"
	"strings"
)

// ParseGroupArg parses "groupID" or "groupID:p1,p2,…" and returns the group ID and a positional params slice.
func ParseGroupArg(arg string) (string, []string, error) {
	groupID, params, err := parsePrimaryArg(arg)
	if err != nil {
		return "", nil, fmt.Errorf("opts.ParseGroupArg: %w", err)
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
