package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jwbonnell/pggosync/config"
	"github.com/stretchr/testify/require"
)

// These tests pin the behaviour the compiler cannot check: that a user's input actually reaches
// the model's fields. Bubble Tea copies the model by value on every Update, while huh binds each
// field to a pointer captured when the form was built — so those bound pointers address a stale
// copy. captureForm reading back through the shared *huh.Form (by key) is the only reason the
// wizard and the config builder work at all; see the C6 entry in docs/remediation-report.md.
//
// Note that huh writes the first option into a bound pointer when a form is built, before the
// model is copied. So assertions must land on a *non-default* value, or they pass even with no
// capture happening at all.

// screenModel is any of the TUI's screen models: each takes a message and returns itself.
type screenModel[T any] interface {
	Update(tea.Msg) (T, tea.Cmd)
}

// tempPathHandler points a UserConfigHandler at a throwaway directory.
type tempPathHandler struct{ dir string }

func (p tempPathHandler) UserConfigDir() (string, error) { return p.dir, nil }

// testStyles is the palette the models are built with under test. The background is never
// queried here, so pick dark — the same provisional value tui.Run starts from.
func testStyles() styles { return newStyles(true) }

// newTestHandler builds a handler over a temp dir, seeded with the named connections.
// ListConnections reads the directory, so the names come back sorted.
func newTestHandler(t *testing.T, conns ...string) *config.UserConfigHandler {
	t.Helper()
	h := config.NewUserConfigHandler(tempPathHandler{dir: t.TempDir()})
	for _, name := range conns {
		require.NoError(t, h.InitConnection(name))
	}
	return h
}

// runCmd executes cmd, giving up if it does not produce a message promptly. Time-based commands
// (the text cursor's blink, spinner ticks) sleep before returning and then renew themselves, so
// waiting on them would add ~500ms per hop and never terminate on its own. They carry no state
// these tests care about. Everything huh uses to advance a form returns immediately.
func runCmd(cmd tea.Cmd) tea.Msg {
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		return msg
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// pump runs cmd and feeds the resulting messages back through Update, the way the Bubble Tea
// runtime would. huh advances and completes a form from a command rather than from inside its
// key handler, so without this a form never reaches StateCompleted and no screen ever moves.
// The depth cap is a backstop against any command chain that renews itself.
func pump[T screenModel[T]](m T, cmd tea.Cmd, depth int) T {
	if cmd == nil || depth > 20 {
		return m
	}
	msg := runCmd(cmd)
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = pump(m, c, depth+1)
		}
		return m
	}
	m, next := m.Update(msg)
	return pump(m, next, depth+1)
}

// press drives one keystroke through Update and settles any resulting commands, mimicking the
// value-copy round trip the real program performs. Returning the new model rather than mutating
// in place is what exposes the stale-pointer trap.
func press[T screenModel[T]](m T, k tea.KeyPressMsg) T {
	next, cmd := m.Update(k)
	return pump(next, cmd, 0)
}

// typeText enters a string one keystroke at a time, the way a terminal delivers it.
//
// Unlike press, it drops the commands the keys return rather than settling them: a printable
// character only ever yields a cursor blink, and waiting out one blink per character would cost
// ~100ms a letter.
func typeText[T screenModel[T]](m T, s string) T {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

var (
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEsc}
	// Bubble Tea v2 reports the space bar as "space" from String(), not " ".
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
)

// ── Sync wizard ─────────────────────────────────────────────────────────────────

// TestSyncWizard_CaptureSource is the core C6 regression: selecting a connection must land in
// selectedSource even though the form's bound pointer targets the copy of the model that
// newSyncWizardModel returned by value.
func TestSyncWizard_CaptureSource(t *testing.T) {
	m := newSyncWizardModel(testStyles(), newTestHandler(t, "alpha", "beta"))
	m = pump(m, m.Init(), 0)

	require.Equal(t, phasePickSource, m.phase)

	// Move off the default option before confirming; "beta" sorts after "alpha".
	m = press(m, keyDown)
	m = press(m, keyEnter)

	require.Equal(t, "beta", m.selectedSource, "form selection did not reach the model field")
	require.Equal(t, phasePickDest, m.phase, "wizard did not advance after the form completed")
}

// TestSyncWizard_CaptureSourceAndDest walks two consecutive connection forms. Both phases bind
// the same "conn" key to different targets, so this catches a capture reading the wrong one.
func TestSyncWizard_CaptureSourceAndDest(t *testing.T) {
	m := newSyncWizardModel(testStyles(), newTestHandler(t, "alpha", "beta"))
	m = pump(m, m.Init(), 0)

	m = press(m, keyEnter) // source = alpha (first option)
	require.Equal(t, phasePickDest, m.phase)

	m = press(m, keyDown)
	m = press(m, keyEnter) // dest = beta

	require.Equal(t, "beta", m.selectedDest)
	require.Equal(t, "alpha", m.selectedSource, "advancing to dest clobbered the source")
	require.Equal(t, phasePickSyncFile, m.phase)
}

// TestSyncWizard_CaptureSyncFilePath pins text-input capture, which takes a different huh path
// (Input rather than Select).
func TestSyncWizard_CaptureSyncFilePath(t *testing.T) {
	m := newSyncWizardModel(testStyles(), newTestHandler(t, "alpha", "beta"))
	m = pump(m, m.Init(), 0)

	m = press(m, keyEnter) // source
	m = press(m, keyEnter) // dest
	require.Equal(t, phasePickSyncFile, m.phase)

	m = typeText(m, "nope")
	m = press(m, keyEnter)

	// The path does not resolve, so the wizard stays put and reports the error — but the typed
	// value must still have been captured off the form.
	require.Equal(t, "nope", m.syncConfigPath, "typed input did not reach the model field")
	require.NotEmpty(t, m.err, "an unresolvable config path should surface an error")
	require.Equal(t, phasePickSyncFile, m.phase, "wizard advanced despite an unresolved config")
}

// TestSyncWizard_GoBackRebuildsPreviousForm pins esc navigation, which Update intercepts before
// huh sees it.
func TestSyncWizard_GoBackRebuildsPreviousForm(t *testing.T) {
	m := newSyncWizardModel(testStyles(), newTestHandler(t, "alpha", "beta"))
	m = pump(m, m.Init(), 0)

	m = press(m, keyEnter)
	require.Equal(t, phasePickDest, m.phase)

	m = press(m, keyEsc)
	require.Equal(t, phasePickSource, m.phase, "esc did not walk back a phase")
}

// TestSyncWizard_BusyGates covers the predicates the root model uses to decide whether ctrl+c
// quits the program or is handed to the wizard to cancel in-flight work. They are trivial, but a
// wrong answer either strands a running sync or kills the app mid-write.
func TestSyncWizard_BusyGates(t *testing.T) {
	m := newSyncWizardModel(testStyles(), newTestHandler(t, "alpha"))
	require.False(t, m.isRunning())
	require.False(t, m.isPreviewLoading())

	m.phase = phaseRunning
	require.True(t, m.isRunning())
	require.False(t, m.isPreviewLoading())

	m.phase = phasePreviewLoading
	require.False(t, m.isRunning())
	require.True(t, m.isPreviewLoading())
}

// ── Sync config builder ─────────────────────────────────────────────────────────

// TestSyncConfigBuilder_CaptureThroughGroup is the exact shape of the original C6 failure: the
// builder used to reject a perfectly good group name with "Group name cannot be empty" because
// the typed value never left the form.
func TestSyncConfigBuilder_CaptureThroughGroup(t *testing.T) {
	m := newSyncConfigModel(testStyles())
	m = pump(m, m.Init(), 0)

	require.Equal(t, scPhaseMain, m.phase)

	m = typeText(m, "my sync config") // description
	m = press(m, keyEnter)            // → exclude
	m = press(m, keyEnter)            // submit main form

	require.Equal(t, "my sync config", m.description, "description did not reach the model")
	require.Equal(t, scPhaseAddGroup, m.phase)

	m = typeText(m, "mygroup")
	m = press(m, keyEnter)

	require.Empty(t, m.err, "a non-empty group name was rejected — captureForm regression")
	require.Equal(t, scPhaseAddTable, m.phase, "builder did not advance to table entry")
	require.Len(t, m.groups, 1)
	require.Equal(t, "mygroup", m.groups[0].name)
}

// TestSyncConfigBuilder_CaptureTable continues into the table form, which binds two inputs.
func TestSyncConfigBuilder_CaptureTable(t *testing.T) {
	m := newSyncConfigModel(testStyles())
	m = pump(m, m.Init(), 0)

	m = press(m, keyEnter) // description (blank)
	m = press(m, keyEnter) // exclude (blank), submit
	m = typeText(m, "mygroup")
	m = press(m, keyEnter)
	require.Equal(t, scPhaseAddTable, m.phase)

	m = typeText(m, "public.users")
	m = press(m, keyEnter) // → filter
	m = typeText(m, "id > 10")
	m = press(m, keyEnter) // submit

	require.Equal(t, scPhaseAddScrub, m.phase)
	require.Len(t, m.groups[0].tables, 1)
	require.Equal(t, "public.users", m.groups[0].tables[0].name, "table name did not reach the model")
	require.Equal(t, "id > 10", m.groups[0].tables[0].filter, "filter did not reach the model")
}

// TestSyncConfigBuilder_EmptyGroupNameRejected pins the validation itself, so the test above
// can't pass simply because validation stopped running.
func TestSyncConfigBuilder_EmptyGroupNameRejected(t *testing.T) {
	m := newSyncConfigModel(testStyles())
	m = pump(m, m.Init(), 0)

	m = press(m, keyEnter) // description (blank)
	m = press(m, keyEnter) // exclude (blank), submit
	require.Equal(t, scPhaseAddGroup, m.phase)

	m = press(m, keyEnter) // submit an empty group name

	require.Equal(t, "Group name cannot be empty", m.err)
	require.Equal(t, scPhaseAddGroup, m.phase, "builder advanced on an empty group name")
	require.Empty(t, m.groups)
}

// ── Menu ────────────────────────────────────────────────────────────────────────

// switchTarget runs the command a menu keystroke produced and reports the screen it asks for.
func switchTarget(t *testing.T, cmd tea.Cmd) (screen, bool) {
	t.Helper()
	if cmd == nil {
		return 0, false
	}
	msg, ok := runCmd(cmd).(switchScreenMsg)
	if !ok {
		return 0, false
	}
	return msg.screen, true
}

// TestMenu_EnterAndSpaceNavigate pins both activation keys. Space matters disproportionately:
// Bubble Tea v2 reports the space bar as "space" rather than " ", so a `case "enter", " "` left
// on the v1 spelling leaves space silently dead with everything still compiling.
func TestMenu_EnterAndSpaceNavigate(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"enter", keyEnter},
		{"space", keySpace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMenuModel(testStyles(), true, nil)
			_, cmd := m.Update(tc.key)

			target, ok := switchTarget(t, cmd)
			require.True(t, ok, "%s did not emit a switchScreenMsg", tc.name)
			require.Equal(t, syncWizardScreen, target, "%s did not open the first menu item", tc.name)
		})
	}
}

// TestSpaceActivatesEveryList covers the other two list screens for the same "space" rename.
// The menu is covered above; these two have no other test reaching their key handler at all.
func TestSpaceActivatesEveryList(t *testing.T) {
	t.Run("user config", func(t *testing.T) {
		// The list always carries the "(+ New connection)" entry first, so space on a fresh
		// screen must open the new-connection form.
		m := newUserConfigModel(testStyles(), true, newTestHandler(t))
		m, _ = m.Update(keySpace)
		require.Equal(t, ucPhaseForm, m.phase, "space did not open the new-connection form")
	})

	t.Run("profiles", func(t *testing.T) {
		h := newTestHandler(t)
		require.NoError(t, h.SaveProfile(config.SyncProfile{Name: "p1", Source: "a", Dest: "b"}))

		m := newProfileModel(testStyles(), true, h)
		_, cmd := m.Update(keySpace)
		require.NotNil(t, cmd, "space did not emit a command")

		msg, ok := runCmd(cmd).(launchProfileMsg)
		require.True(t, ok, "space did not emit a launchProfileMsg")
		require.Equal(t, "p1", msg.profile.Name)
	})
}

// TestMenu_SelectionFollowsCursor confirms the emitted target tracks the highlighted item,
// rather than being hardcoded to the first.
func TestMenu_SelectionFollowsCursor(t *testing.T) {
	m := newMenuModel(testStyles(), true, nil)
	m, _ = m.Update(keyDown)
	_, cmd := m.Update(keyEnter)

	target, ok := switchTarget(t, cmd)
	require.True(t, ok, "enter did not emit a switchScreenMsg")
	require.Equal(t, userConfigScreen, target, "enter opened the wrong menu item")
}
