package tui

import (
	"testing"

	"github.com/jwbonnell/pggosync/config"
	"github.com/stretchr/testify/require"
)

// The .Value(&ptr) bindings on the wizard's fields look redundant next to captureForm, which
// re-reads every value by key anyway. They are not: the two move data in opposite directions.
//
//	.Value(&ptr) → huh reads *ptr when the form is built, to seed the field's initial state
//	               (Select.Accessor calls selectValue(accessor.Get())).
//	captureForm  → reads back out of the form by key, because huh writes through those same
//	               pointers into a stale copy of the model — see the C6 note in
//	               model_capture_test.go.
//
// Deleting the "redundant" bindings compiles cleanly and silently drops every default: a launched
// profile's saved options stop appearing in the options form. These tests make that loud.
//
// Note huh only records a value into the form's results map as the user advances off a field, so
// a freshly built form reads back empty — the seeded value has to be stepped past to be observed.

// TestOptionsFormSeedsStrategyFromProfile is the discriminating case: "upsert" is both the first
// option and the zero value, so the assertion is on "truncate" — a value the form can only show
// if it was seeded from the profile.
func TestOptionsFormSeedsStrategyFromProfile(t *testing.T) {
	m := newSyncWizardModelFromProfile(testStyles(), newTestHandler(t, "alpha", "beta"),
		config.SyncProfile{Name: "prof", Source: "alpha", Dest: "beta", Truncate: true, Concurrency: 1})

	// Reaching the options form is what a user does by backing out of a launched profile's
	// preview: it rebuilds the form from the options the profile loaded.
	m.phase = phasePickOptions
	m.form = m.buildOptionsForm()

	// Step off the strategy field (without changing it) so huh records what it was holding.
	m = press(m, keyEnter)

	require.Equal(t, "truncate", m.form.GetString("strategy"),
		"the profile's strategy did not seed the form — are the .Value bindings still there?")
	require.Equal(t, phasePickOptions, m.phase, "the form should not have completed yet")
}

func TestOptionsFormSeedsPreserveFromProfile(t *testing.T) {
	m := newSyncWizardModelFromProfile(testStyles(), newTestHandler(t, "alpha", "beta"),
		config.SyncProfile{Name: "prof", Source: "alpha", Dest: "beta", Preserve: true, Concurrency: 1})

	m.phase = phasePickOptions
	m.form = m.buildOptionsForm()
	m = press(m, keyEnter)

	require.Equal(t, "preserve", m.form.GetString("strategy"),
		"buildOptionsForm derives the strategy select from two separate booleans; preserve was lost")
}

// TestOptionsFormSeedsBoolFromProfile covers a Confirm field, which seeds through a different
// huh path (a bare accessor) than Select does.
func TestOptionsFormSeedsBoolFromProfile(t *testing.T) {
	m := newSyncWizardModelFromProfile(testStyles(), newTestHandler(t, "alpha", "beta"),
		config.SyncProfile{
			Name: "prof", Source: "alpha", Dest: "beta",
			Truncate: true, Cascade: true, Concurrency: 1,
		})

	m.phase = phasePickOptions
	m.form = m.buildOptionsForm()

	// strategy → cascade: two fields to step past.
	m = press(m, keyEnter)
	m = press(m, keyEnter)

	require.True(t, m.form.GetBool("cascade"), "the profile's cascade did not seed the form")
}
