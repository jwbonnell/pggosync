package cmd

import "github.com/charmbracelet/lipgloss"

// Matrix-green palette mirrored from the TUI (see tui/styles.go) so that non-interactive
// CLI output matches the interactive UI. Kept as a small local copy rather than importing
// the tui package (which would pull bubbletea/huh into every CLI command). AdaptiveColor
// keeps the text legible on both light and dark terminals; lipgloss renders plain,
// un-escaped text automatically when stdout is not a TTY or NO_COLOR is set.
var (
	cliGreen  = lipgloss.AdaptiveColor{Light: "#047A34", Dark: "#00FF41"} // brand / art / values
	cliSubtle = lipgloss.AdaptiveColor{Light: "#3F6B50", Dark: "#57B36E"} // labels
	cliMuted  = lipgloss.AdaptiveColor{Light: "#5B8C6E", Dark: "#2F7A45"} // dim / false values
	cliText   = lipgloss.AdaptiveColor{Light: "#14331F", Dark: "#C8FFD4"} // body text
)

var (
	bannerArtStyle   = lipgloss.NewStyle().Foreground(cliGreen).Bold(true)
	bannerLabelStyle = lipgloss.NewStyle().Foreground(cliSubtle)
	bannerTextStyle  = lipgloss.NewStyle().Foreground(cliText)
	bannerOnStyle    = lipgloss.NewStyle().Foreground(cliGreen).Bold(true)
	bannerOffStyle   = lipgloss.NewStyle().Foreground(cliMuted)
)

// styledBool renders a boolean in the matrix palette: bright green when true, dim when false.
func styledBool(b bool) string {
	if b {
		return bannerOnStyle.Render("true")
	}
	return bannerOffStyle.Render("false")
}
