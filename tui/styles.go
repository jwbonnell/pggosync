package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// ── Palette ─────────────────────────────────────────────────────────────────────
//
// Semantic colors for the whole TUI. Each is an AdaptiveColor so the UI reads well
// on both light- and dark-background terminals; lipgloss downsamples the truecolor
// hexes automatically on terminals with limited color support.
// Monochrome Matrix scheme — every token is a shade of green. Dark variants are bright CRT
// greens for the classic black-terminal look; Light variants drop to dark forest greens so
// text stays legible on a light background. Since hue no longer separates states, errors are
// distinguished by weight (bold) rather than color — see errorStyle / indicatorFailed below.
var (
	colorPrimary    = lipgloss.AdaptiveColor{Light: "#047A34", Dark: "#00FF41"} // brand / titles / selection
	colorSecondary  = lipgloss.AdaptiveColor{Light: "#0E7A4A", Dark: "#00CC66"} // info / prefetch
	colorSuccess    = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#39FF7A"} // done
	colorWarning    = lipgloss.AdaptiveColor{Light: "#3F6B0E", Dark: "#7CFF4D"} // caution
	colorError      = lipgloss.AdaptiveColor{Light: "#0A5A28", Dark: "#00C853"} // failed (dim green + bold)
	colorMuted      = lipgloss.AdaptiveColor{Light: "#5B8C6E", Dark: "#2F7A45"} // help / borders / queued
	colorSubtle     = lipgloss.AdaptiveColor{Light: "#3F6B50", Dark: "#57B36E"} // secondary text
	colorText       = lipgloss.AdaptiveColor{Light: "#14331F", Dark: "#C8FFD4"} // primary body text
	colorScrub      = lipgloss.AdaptiveColor{Light: "#0E7A4A", Dark: "#00FFA3"} // scrub badge (spring green)
	colorSelectedBg = lipgloss.AdaptiveColor{Light: "#DCFCE7", Dark: "#0B3D1A"} // selected-row background
)

// Gradient endpoints for the sync progress bar. progress.WithGradient wants concrete
// color strings, so these use dark-terminal green hexes (bright → deep phosphor green).
const (
	gradientStart = "#00FF66"
	gradientEnd   = "#00994D"
)

// ── Shared styles ───────────────────────────────────────────────────────────────
//
// Every screen draws from this set so the look stays consistent. Defining them once
// here (rather than inline per file) is what keeps the palette cohesive.
var (
	// Chrome shared across list screens.
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Padding(0, 1)
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	lastSyncStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// Wizard / config / results chrome.
	wizardTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).MarginBottom(1)
	errorStyle       = lipgloss.NewStyle().Foreground(colorError).Bold(true).MarginTop(1)
	helpStyle        = lipgloss.NewStyle().Foreground(colorMuted).MarginTop(1)
	successStyle     = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	mutedStyle       = lipgloss.NewStyle().Foreground(colorMuted)

	borderStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorMuted).Padding(0, 1).Margin(1, 2)
	detailBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorPrimary).Padding(1, 2)
	detailTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).MarginBottom(1)
	detailKeyStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	detailValueStyle  = lipgloss.NewStyle().Foreground(colorText)
	selectedRowStyle  = lipgloss.NewStyle().Background(colorSelectedBg)
	scrubStyle        = lipgloss.NewStyle().Foreground(colorScrub).Bold(true)

	// Per-table status indicators for the running view. Kept margin-free so they can be
	// rendered inline next to a single glyph without disturbing the layout.
	indicatorDone     = lipgloss.NewStyle().Foreground(colorSuccess)
	indicatorFailed   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	indicatorWriting  = lipgloss.NewStyle().Foreground(colorPrimary)
	indicatorPrefetch = lipgloss.NewStyle().Foreground(colorSecondary)
	indicatorQueued   = lipgloss.NewStyle().Foreground(colorMuted)
)

// statusStyle maps a per-table sync phase to its indicator style, so View() never has
// to allocate styles in the render hot path.
func statusStyle(phase tablePhase) lipgloss.Style {
	switch phase {
	case tableDone:
		return indicatorDone
	case tableFailed:
		return indicatorFailed
	case tableWriting:
		return indicatorWriting
	case tablePrefetching, tablePrefetchReady:
		return indicatorPrefetch
	default:
		return indicatorQueued
	}
}

// ── Form theme ──────────────────────────────────────────────────────────────────

// newForm builds a huh form pre-styled with the app's formTheme. Use it in place of
// huh.NewForm everywhere so every form matches the rest of the TUI.
func newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(formTheme())
}

// formTheme returns a huh theme recolored to the palette above, so every form matches
// the rest of the app instead of using huh's default charm colors. Built on ThemeBase
// and recolored in the same shape as huh's built-in ThemeCharm.
func formTheme() *huh.Theme {
	t := huh.ThemeBase()

	// Text color for buttons/selections drawn on the bright-green primary: near-black on
	// dark terminals (green is the background), white on light terminals (green is dark).
	onPrimary := lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#00160B"}

	t.Focused.Base = t.Focused.Base.BorderForeground(colorMuted)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(colorPrimary).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(colorPrimary).Bold(true).MarginBottom(1)
	t.Focused.Directory = t.Focused.Directory.Foreground(colorPrimary)
	t.Focused.Description = t.Focused.Description.Foreground(colorSubtle)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(colorError)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(colorError)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(colorPrimary)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(colorPrimary)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(colorPrimary)
	t.Focused.Option = t.Focused.Option.Foreground(colorText)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(colorPrimary)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colorSuccess)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(colorSuccess).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(colorSubtle).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(colorText)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(onPrimary).Background(colorPrimary)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(colorText).Background(colorSelectedBg)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colorSecondary)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(colorMuted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(colorPrimary)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}
