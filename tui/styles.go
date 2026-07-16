package tui

import (
	"image/color"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// ── Palette ─────────────────────────────────────────────────────────────────────
//
// Semantic colors for the whole TUI, as light/dark pairs resolved by newStyles.
// Monochrome Matrix scheme — every token is a shade of green. Dark variants are bright CRT
// greens for the classic black-terminal look; Light variants drop to dark forest greens so
// text stays legible on a light background. Since hue no longer separates states, errors are
// distinguished by weight (bold) rather than color — see failure / indicatorFailed below.
//
// lipgloss v2 dropped AdaptiveColor: nothing resolves the terminal background for us any more.
// So the pairs stay inert here and newStyles(isDark) collapses them into concrete colors. The
// TUI learns isDark by asking the terminal (tea.RequestBackgroundColor, see tui.go); the
// connection form, which also runs standalone outside Bubble Tea, asks lipgloss directly.
var (
	huePrimary    = lightDark{light: "#047A34", dark: "#00FF41"} // brand / titles / selection
	hueSecondary  = lightDark{light: "#0E7A4A", dark: "#00CC66"} // info / prefetch
	hueSuccess    = lightDark{light: "#15803D", dark: "#39FF7A"} // done
	hueFailure    = lightDark{light: "#0A5A28", dark: "#00C853"} // failed (dim green + bold)
	hueMuted      = lightDark{light: "#5B8C6E", dark: "#2F7A45"} // help / borders / queued
	hueSubtle     = lightDark{light: "#3F6B50", dark: "#57B36E"} // secondary text
	hueText       = lightDark{light: "#14331F", dark: "#C8FFD4"} // primary body text
	hueScrub      = lightDark{light: "#0E7A4A", dark: "#00FFA3"} // scrub badge (spring green)
	hueSelectedBg = lightDark{light: "#DCFCE7", dark: "#0B3D1A"} // selected-row background

	// Text drawn on top of the bright-green primary: near-black on dark terminals (where
	// primary is the background), white on light ones (where primary is dark).
	hueOnPrimary = lightDark{light: "#FFFFFF", dark: "#00160B"}
)

// lightDark is a color pair awaiting a terminal background.
type lightDark struct{ light, dark string }

func (p lightDark) resolve(pick lipgloss.LightDarkFunc) color.Color {
	return pick(lipgloss.Color(p.light), lipgloss.Color(p.dark))
}

// palette is the set of colors above, resolved for one terminal background. The list delegate,
// the spinner and the form theme all want a color rather than a finished Style, so the resolved
// set is kept around rather than being discarded once the styles are built.
type palette struct {
	primary    color.Color
	secondary  color.Color
	success    color.Color
	failure    color.Color
	muted      color.Color
	subtle     color.Color
	text       color.Color
	scrub      color.Color
	selectedBg color.Color
	onPrimary  color.Color
}

// Gradient endpoints for the sync progress bar (bright → deep phosphor green). The bar keeps
// its dark-terminal greens on both backgrounds, so these are not a light/dark pair.
const (
	gradientStart = "#00FF66"
	gradientEnd   = "#00994D"
)

// ── Shared styles ───────────────────────────────────────────────────────────────
//
// Every screen draws from this set so the look stays consistent. Building them once (rather
// than inline per file) is what keeps the palette cohesive. The set is threaded through the
// screen models rather than kept in package-level vars, because it depends on the terminal
// background, which is not known until the terminal answers.
type styles struct {
	c palette

	// Chrome shared across list screens.
	title    lipgloss.Style
	doc      lipgloss.Style
	lastSync lipgloss.Style

	// Wizard / config / results chrome.
	wizardTitle lipgloss.Style
	err         lipgloss.Style
	help        lipgloss.Style
	success     lipgloss.Style
	muted       lipgloss.Style

	border       lipgloss.Style
	detailBorder lipgloss.Style
	detailTitle  lipgloss.Style
	detailKey    lipgloss.Style
	detailValue  lipgloss.Style
	selectedRow  lipgloss.Style
	scrub        lipgloss.Style

	// Per-table status indicators for the running view. Kept margin-free so they can be
	// rendered inline next to a single glyph without disturbing the layout.
	indicatorDone     lipgloss.Style
	indicatorFailed   lipgloss.Style
	indicatorWriting  lipgloss.Style
	indicatorPrefetch lipgloss.Style
	indicatorQueued   lipgloss.Style
}

// newStyles resolves the palette against the terminal background and builds every shared style.
// Call it once per background change, never per render.
func newStyles(isDark bool) styles {
	pick := lipgloss.LightDark(isDark)

	c := palette{
		primary:    huePrimary.resolve(pick),
		secondary:  hueSecondary.resolve(pick),
		success:    hueSuccess.resolve(pick),
		failure:    hueFailure.resolve(pick),
		muted:      hueMuted.resolve(pick),
		subtle:     hueSubtle.resolve(pick),
		text:       hueText.resolve(pick),
		scrub:      hueScrub.resolve(pick),
		selectedBg: hueSelectedBg.resolve(pick),
		onPrimary:  hueOnPrimary.resolve(pick),
	}

	return styles{
		c: c,

		title:    lipgloss.NewStyle().Bold(true).Foreground(c.primary).Padding(0, 1),
		doc:      lipgloss.NewStyle().Margin(1, 2),
		lastSync: lipgloss.NewStyle().Foreground(c.muted),

		wizardTitle: lipgloss.NewStyle().Bold(true).Foreground(c.primary).MarginBottom(1),
		err:         lipgloss.NewStyle().Foreground(c.failure).Bold(true).MarginTop(1),
		help:        lipgloss.NewStyle().Foreground(c.muted).MarginTop(1),
		success:     lipgloss.NewStyle().Foreground(c.success).Bold(true),
		muted:       lipgloss.NewStyle().Foreground(c.muted),

		border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.muted).Padding(0, 1).Margin(1, 2),
		detailBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.primary).Padding(1, 2),
		detailTitle:  lipgloss.NewStyle().Bold(true).Foreground(c.primary).MarginBottom(1),
		detailKey:    lipgloss.NewStyle().Foreground(c.muted),
		detailValue:  lipgloss.NewStyle().Foreground(c.text),
		selectedRow:  lipgloss.NewStyle().Background(c.selectedBg),
		scrub:        lipgloss.NewStyle().Foreground(c.scrub).Bold(true),

		indicatorDone:     lipgloss.NewStyle().Foreground(c.success),
		indicatorFailed:   lipgloss.NewStyle().Foreground(c.failure).Bold(true),
		indicatorWriting:  lipgloss.NewStyle().Foreground(c.primary),
		indicatorPrefetch: lipgloss.NewStyle().Foreground(c.secondary),
		indicatorQueued:   lipgloss.NewStyle().Foreground(c.muted),
	}
}

// statusStyle maps a per-table sync phase to its indicator style, so View() never has
// to allocate styles in the render hot path.
func (s styles) statusStyle(phase tablePhase) lipgloss.Style {
	switch phase {
	case tableDone:
		return s.indicatorDone
	case tableFailed:
		return s.indicatorFailed
	case tableWriting:
		return s.indicatorWriting
	case tablePrefetching, tablePrefetchReady:
		return s.indicatorPrefetch
	default:
		return s.indicatorQueued
	}
}

// ── Form theme ──────────────────────────────────────────────────────────────────

// newForm builds a huh form pre-styled with the app's formTheme. Prefer each model's own newForm,
// which also sizes the result; use this directly only where no terminal width is known.
func (s styles) newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(s.formTheme))
}

// sizeForm gives a freshly built form the terminal width.
//
// huh derives a form's width from the first tea.WindowSizeMsg it sees, but only while that width
// is still unset — and this app builds a new form on every phase change, long after that message
// has come and gone. Without this, every form after the first renders at huh's 80-column default
// until the user happens to resize the terminal. Setting the width also means huh stops adopting
// resizes for us, so each model re-applies it from its own WindowSizeMsg handler.
//
// Width only, deliberately: huh's resize path fits height to the content (min(needed, terminal)),
// whereas WithHeight would force every group to the full terminal height.
func sizeForm(f *huh.Form, width int) *huh.Form {
	if width <= 0 {
		return f
	}
	return f.WithWidth(width)
}

// formTheme returns a huh theme recolored to the palette above, so every form matches the rest
// of the app instead of using huh's default charm colors. Built on ThemeBase and recolored in
// the same shape as huh's built-in ThemeCharm.
//
// The isDark huh passes is deliberately ignored in favour of the receiver's already-resolved
// palette. huh tracks the background per-Form, defaulting to false until a tea.BackgroundColorMsg
// reaches that Form — but this app rebuilds its form on every phase change, so a form built after
// startup would never see one and would render light on a dark terminal. The app resolves the
// background once (see tui.go) and every form inherits it, whenever it happens to be built.
func (s styles) formTheme(bool) *huh.Styles {
	// ThemeBase's own isDark only picks defaults that are overwritten below.
	t := huh.ThemeBase(false)

	t.Focused.Base = t.Focused.Base.BorderForeground(s.c.muted)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(s.c.primary).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(s.c.primary).Bold(true).MarginBottom(1)
	t.Focused.Directory = t.Focused.Directory.Foreground(s.c.primary)
	t.Focused.Description = t.Focused.Description.Foreground(s.c.subtle)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(s.c.failure)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(s.c.failure)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(s.c.primary)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(s.c.primary)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(s.c.primary)
	t.Focused.Option = t.Focused.Option.Foreground(s.c.text)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(s.c.primary)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(s.c.success)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(s.c.success).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(s.c.subtle).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(s.c.text)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(s.c.onPrimary).Background(s.c.primary)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(s.c.text).Background(s.c.selectedBg)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(s.c.secondary)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(s.c.muted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(s.c.primary)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}
