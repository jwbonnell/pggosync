package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

// huh lays a form out from the first tea.WindowSizeMsg it sees, and only while the form's width is
// still unset. The wizard builds a brand new form on every phase change — long after that message
// has come and gone — so a form built mid-flow used to fall back to huh's 80-column default and
// stay there until the user happened to resize the terminal. sizeForm hands each new form the
// known width; because that also stops huh adopting resizes on its own, each model re-applies the
// width from its own WindowSizeMsg handler. Both halves are covered here.

// renderedWidth is the width of the widest line the form renders, ignoring ANSI styling.
func renderedWidth(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if n := lipgloss.Width(line); n > w {
			w = n
		}
	}
	return w
}

const huhDefaultWidth = 80

func TestFormsAreSizedToTheTerminal(t *testing.T) {
	m := newSyncWizardModel(testStyles(), newTestHandler(t, "alpha", "beta"))
	m = pump(m, m.Init(), 0)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	require.Equal(t, 120, renderedWidth(m.form.View()), "the first form ignored the terminal size")

	// A new form, built with no resize following it: the case that used to regress to 80.
	m = press(m, keyEnter)
	require.Equal(t, phasePickDest, m.phase)
	require.Equal(t, 120, renderedWidth(m.form.View()),
		"a form built mid-flow fell back to huh's %d-column default", huhDefaultWidth)

	// Pinning the width must not cost us resize tracking.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	require.Equal(t, 100, renderedWidth(m.form.View()), "the form did not follow a resize")

	// And a form built after that resize picks up the new width.
	m = press(m, keyEnter)
	require.Equal(t, phasePickSyncFile, m.phase)
	require.Equal(t, 100, renderedWidth(m.form.View()), "a form built after a resize used a stale width")
}

// TestConfigBuilderFormsAreSized covers the other screen that rebuilds a form per phase.
func TestConfigBuilderFormsAreSized(t *testing.T) {
	m := newSyncConfigModel(testStyles())
	m = pump(m, m.Init(), 0)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	require.Equal(t, 120, renderedWidth(m.form.View()))

	// main → add-group builds a fresh form.
	m = press(m, keyEnter) // description
	m = press(m, keyEnter) // exclude, submits
	require.Equal(t, scPhaseAddGroup, m.phase)
	require.Equal(t, 120, renderedWidth(m.form.View()),
		"the group form fell back to huh's %d-column default", huhDefaultWidth)
}

// TestUnsizedFormKeepsHuhDefault pins the guard in sizeForm: before any resize arrives the width
// is still zero, and huh must be left to its own default rather than being handed a zero width.
func TestUnsizedFormKeepsHuhDefault(t *testing.T) {
	m := newSyncWizardModel(testStyles(), newTestHandler(t, "alpha", "beta"))
	m = pump(m, m.Init(), 0)

	require.Equal(t, huhDefaultWidth, renderedWidth(m.form.View()),
		"a form built before any resize should keep huh's own default")
}
