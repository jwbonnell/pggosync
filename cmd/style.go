package cmd

import (
	"os"
	"sync"

	"charm.land/lipgloss/v2"
)

// Matrix-green palette mirrored from the TUI (see tui/styles.go) so that non-interactive
// CLI output matches the interactive UI. Kept as a small local copy rather than importing
// the tui package (which would pull bubbletea/huh into every CLI command).
//
// Two lipgloss v2 traps are worth spelling out, because both fail silently:
//
//   - AdaptiveColor is gone, and nothing detects the terminal background for us any more. The
//     light/dark pairs below are resolved once, on first use. Unlike the TUI these commands are
//     not a Bubble Tea program, so there is no tea.BackgroundColorMsg to learn it from and the
//     terminal has to be asked directly.
//   - Style.Render no longer drops colour for non-TTY output. It always emits full-fidelity
//     ANSI, and downsampling moved to print time — so styled text must be printed with
//     lipgloss.Print*/Fprint*, never fmt.Print*, or escape codes leak into pipes, files and
//     NO_COLOR runs.
type cliPalette struct {
	art    lipgloss.Style
	label  lipgloss.Style
	text   lipgloss.Style
	on     lipgloss.Style
	off    lipgloss.Style
	failed lipgloss.Style
}

var (
	cliStylesOnce sync.Once
	cliStyles     cliPalette
)

// palette returns the banner styles, resolving the terminal background on first call.
//
// The query is deferred rather than run at package init because these commands include paths
// where asking the terminal anything is pointless (--output json, piped output) and every CLI
// invocation imports this file. lipgloss.HasDarkBackground short-circuits when stdin/stdout are
// not a terminal and defaults to dark, so a piped run neither hangs nor waits on a reply that
// will never arrive.
func palette() cliPalette {
	cliStylesOnce.Do(func() {
		pick := lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))

		green := pick(lipgloss.Color("#047A34"), lipgloss.Color("#00FF41"))  // brand / art / values
		subtle := pick(lipgloss.Color("#3F6B50"), lipgloss.Color("#57B36E")) // labels
		muted := pick(lipgloss.Color("#5B8C6E"), lipgloss.Color("#2F7A45"))  // dim / false values
		text := pick(lipgloss.Color("#14331F"), lipgloss.Color("#C8FFD4"))   // body text
		red := pick(lipgloss.Color("#B00020"), lipgloss.Color("#FF5555"))    // failures / mismatches

		cliStyles = cliPalette{
			art:    lipgloss.NewStyle().Foreground(green).Bold(true),
			label:  lipgloss.NewStyle().Foreground(subtle),
			text:   lipgloss.NewStyle().Foreground(text),
			on:     lipgloss.NewStyle().Foreground(green).Bold(true),
			off:    lipgloss.NewStyle().Foreground(muted),
			failed: lipgloss.NewStyle().Foreground(red).Bold(true),
		}
	})
	return cliStyles
}

func bannerArtStyle() lipgloss.Style   { return palette().art }
func bannerLabelStyle() lipgloss.Style { return palette().label }
func bannerTextStyle() lipgloss.Style  { return palette().text }
func bannerOnStyle() lipgloss.Style    { return palette().on }
func bannerOffStyle() lipgloss.Style   { return palette().off }
func bannerFailStyle() lipgloss.Style  { return palette().failed }

// styledBool renders a boolean in the matrix palette: bright green when true, dim when false.
func styledBool(b bool) string {
	if b {
		return bannerOnStyle().Render("true")
	}
	return bannerOffStyle().Render("false")
}
