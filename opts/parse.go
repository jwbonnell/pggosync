package opts

import (
	"errors"
	"fmt"
	"strings"
)

func ParseGroupArg(arg string) (string, []string, error) {
	groupID, params, err := parsePrimaryArg(arg)
	if err != nil {
		return "", nil, fmt.Errorf("opts.ParseGroupArg: %w", err)
	}
	return groupID, strings.Split(params, ","), nil
}

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
