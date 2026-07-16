package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/jwbonnell/pggosync/config"
)

// connectionFormValues holds the string-typed form fields for creating or editing a connection.
type connectionFormValues struct {
	Name     string
	Host     string
	Port     string
	Database string
	User     string
	Password string
	SSLMode  string
}

// newConnectionFormValues seeds form values from an existing connection, or with defaults when existing is nil.
func newConnectionFormValues(name string, existing *config.ConnectionConfig) *connectionFormValues {
	if existing == nil {
		return &connectionFormValues{
			Host:     "localhost",
			Port:     "5444",
			Database: "postgres",
			SSLMode:  "disable",
		}
	}
	v := &connectionFormValues{
		Name:     name,
		Host:     existing.Host,
		Port:     strconv.Itoa(existing.Port),
		Database: existing.Database,
		User:     existing.User,
		Password: existing.Password,
		SSLMode:  existing.SSLMode,
	}
	if v.SSLMode == "" {
		v.SSLMode = "disable"
	}
	return v
}

// connection converts the form values into a name and ConnectionConfig.
func (v *connectionFormValues) connection() (string, config.ConnectionConfig) {
	port, _ := strconv.Atoi(strings.TrimSpace(v.Port))
	return strings.TrimSpace(v.Name), config.ConnectionConfig{
		Host:     v.Host,
		Port:     port,
		Database: v.Database,
		User:     v.User,
		Password: v.Password,
		SSLMode:  v.SSLMode,
	}
}

// newConnectionForm builds the Huh form bound to v; placeholderName is shown on the
// name field when editing an existing connection. nameValidate, when non-nil, is an
// extra check run against the (trimmed) name — used to reject names that would
// overwrite an existing connection.
func newConnectionForm(s styles, v *connectionFormValues, placeholderName string, nameValidate func(string) error) *huh.Form {
	nameField := huh.NewInput().
		Title("Connection name").
		Description("Identifier for this connection").
		Validate(func(s string) error {
			s = strings.TrimSpace(s)
			if s == "" {
				return fmt.Errorf("connection name is required")
			}
			if nameValidate != nil {
				return nameValidate(s)
			}
			return nil
		}).
		Value(&v.Name)
	if placeholderName != "" {
		nameField = nameField.Placeholder(placeholderName)
	}
	return s.newForm(
		huh.NewGroup(
			nameField,
			huh.NewInput().Title("Host").Value(&v.Host),
			huh.NewInput().Title("Port").Value(&v.Port).Validate(func(s string) error {
				p, err := strconv.Atoi(strings.TrimSpace(s))
				if err != nil || p < 1 || p > 65535 {
					return fmt.Errorf("must be a number between 1 and 65535")
				}
				return nil
			}),
			huh.NewInput().Title("Database").Value(&v.Database),
			huh.NewInput().Title("User").Value(&v.User),
			huh.NewInput().Title("Password").EchoMode(huh.EchoModePassword).Value(&v.Password),
			huh.NewSelect[string]().
				Title("SSL mode").
				Options(
					huh.NewOption("disable", "disable"),
					huh.NewOption("prefer", "prefer"),
					huh.NewOption("require", "require"),
					huh.NewOption("verify-full", "verify-full"),
				).
				Value(&v.SSLMode),
		),
	)
}

// RunConnectionForm runs the connection form standalone (outside the full TUI),
// saves the resulting connection, and returns its name. Aborting the form
// returns huh.ErrUserAborted.
//
// Running outside Bubble Tea means there is no tea.BackgroundColorMsg to learn the terminal
// background from (as tui.Run does), so the background is queried directly here. The query
// short-circuits when stdin/stdout are not a terminal, defaulting to dark.
func RunConnectionForm(handler *config.UserConfigHandler) (string, error) {
	s := newStyles(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
	v := newConnectionFormValues("", nil)
	rejectExisting := func(name string) error {
		exists, err := handler.ConnectionExists(name)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("connection %q already exists", name)
		}
		return nil
	}
	if err := newConnectionForm(s, v, "", rejectExisting).Run(); err != nil {
		return "", err
	}
	name, conn := v.connection()
	if err := handler.SaveConnection(name, conn); err != nil {
		return "", err
	}
	return name, nil
}
