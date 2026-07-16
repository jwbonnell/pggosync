package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

// The palette is resolved by asking the terminal for its background (Init sends
// tea.RequestBackgroundColor) and rebuilding the styles when it answers. Nothing else exercises
// that path: the program starts on the dark variants, so a broken reply handler looks perfectly
// fine on the dark terminals this is usually developed against, and only shows up as an
// unreadable UI for someone on a light one.

// screenStyles pulls the styles each screen is currently holding. The root model has to push a
// rebuilt palette into all five — they are constructed up front, and switching to one does not
// reconstruct it — so a screen missing from the fan-out keeps the provisional dark styles forever.
func screenStyles(m model) map[string]styles {
	return map[string]styles{
		"menu":       m.menu.styles,
		"syncWizard": m.syncWizard.styles,
		"userConfig": m.userConfig.styles,
		"syncConfig": m.syncConfig.styles,
		"profiles":   m.profiles.styles,
	}
}

func TestBackgroundColorResolvesPalette(t *testing.T) {
	// Guard the premise: if light and dark resolved the same, every assertion below would pass
	// while proving nothing.
	require.NotEqual(t, newStyles(true), newStyles(false),
		"light and dark palettes are identical — the light/dark pairs are not being resolved")

	tests := []struct {
		name   string
		bg     string
		isDark bool
	}{
		{"white background resolves light", "#FFFFFF", false},
		{"black background resolves dark", "#000000", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(newTestHandler(t, "alpha"))
			require.True(t, m.isDark, "the model should start on the provisional dark palette")

			next, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color(tc.bg)})
			got := next.(model)

			require.Equal(t, tc.isDark, got.isDark)

			want := newStyles(tc.isDark)
			require.Equal(t, want, got.styles, "root model kept the wrong palette")
			for name, s := range screenStyles(got) {
				require.Equal(t, want, s, "%s screen was not re-themed", name)
			}
		})
	}
}

// TestFormThemeIgnoresHuhIsDark pins a deliberate oddity that reads like a bug and would
// otherwise be "fixed" back into one.
//
// huh calls the ThemeFunc with a background it tracks per-Form, defaulting to false until a
// tea.BackgroundColorMsg reaches that Form. This app rebuilds its form on every phase change, so
// forms built after startup never see one — honouring huh's argument would render them light on
// a dark terminal. formTheme therefore ignores it and uses the already-resolved palette.
func TestFormThemeIgnoresHuhIsDark(t *testing.T) {
	for _, isDark := range []bool{true, false} {
		s := newStyles(isDark)
		require.Equal(t, s.formTheme(true), s.formTheme(false),
			"formTheme must ignore the isDark huh passes and use its own palette (styles isDark=%v)", isDark)
	}

	// ...and the palette it uses is the resolved one, so the two backgrounds still differ.
	require.NotEqual(t, newStyles(true).formTheme(false), newStyles(false).formTheme(false),
		"the form theme does not follow the resolved palette at all")
}

// TestBackgroundColorRethemesForms covers the whole chain: a background reply must reach the
// theme that huh actually renders the wizard's form through.
func TestBackgroundColorRethemesForms(t *testing.T) {
	m := newModel(newTestHandler(t, "alpha"))
	darkTheme := m.styles.formTheme(true)

	next, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#FFFFFF")})
	got := next.(model)

	require.Equal(t, newStyles(false).formTheme(true), got.syncWizard.styles.formTheme(true),
		"the wizard's forms would still be themed for a dark terminal")
	require.NotEqual(t, darkTheme, got.syncWizard.styles.formTheme(true),
		"the form theme did not change at all")
}
