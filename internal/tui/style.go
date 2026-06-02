package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette — dark terminal theme from docs/commands.html
const (
	ColorAccent      = "#fb923c"
	ColorAccentLight = "#fdba74"
	ColorOK          = "#86efac"
	ColorOKDim       = "#4ade80"
	ColorError       = "#fca5a5"
	ColorWarning     = "#fcd34d"
	ColorInfo        = "#93c5fd"
	ColorText        = "#e7e5e4"
	ColorTextMuted   = "#a8a29e"
	ColorTextDim     = "#78716c"
	ColorBorder      = "#44403c"
	ColorSurface     = "#292524"
	ColorBg          = "#1c1917"
	ColorDeep        = "#0c0a09"
)

var (
	Brand   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent))
	Accent  = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccentLight))
	OK      = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOK))
	OKDim   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOKDim))
	Error   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError))
	Warning = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorWarning))
	Info    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorInfo))
	Text    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorText))
	Muted   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorTextMuted))
	Dim     = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorTextDim))
	URL     = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccentLight)).UnderlineSpaces(true)

	BadgeOK = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorOK)).
		Background(lipgloss.Color("#1a2e1a")).
		Padding(0, 1).
		Bold(true)

	BadgeError = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorError)).
		Background(lipgloss.Color("#2e1a1a")).
		Padding(0, 1).
		Bold(true)

	BadgeWarning = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWarning)).
		Background(lipgloss.Color("#2e2a1a")).
		Padding(0, 1).
		Bold(true)

	BadgeInfo = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccentLight)).
		Background(lipgloss.Color("#2e231a")).
		Padding(0, 1).
		Bold(true)

	BadgeDim = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorTextDim)).
		Background(lipgloss.Color("#2a2420")).
		Padding(0, 1).
		Bold(true)

	BoldOK = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorOK)).
		Bold(true)

	Box = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorBorder)).
		Padding(0, 1).
		MarginTop(0).
		MarginBottom(0)

	BoxWithTitle = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorBorder)).
		Padding(1, 2)

	Divider = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorBorder))

	TableHeader = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWarning)).
		Bold(true)

	Prompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWarning)).
		Bold(true)
)

// Sprint helpers
func BrandS(s string) string   { return Brand.Render(s) }
func AccentS(s string) string  { return Accent.Render(s) }
func OKS(s string) string      { return OK.Render(s) }
func OKDimS(s string) string   { return OKDim.Render(s) }
func ErrorS(s string) string  { return Error.Render(s) }
func WarningS(s string) string { return Warning.Render(s) }
func InfoS(s string) string    { return Info.Render(s) }
func TextS(s string) string    { return Text.Render(s) }
func MutedS(s string) string   { return Muted.Render(s) }
func DimS(s string) string     { return Dim.Render(s) }
func BoldOKS(s string) string  { return BoldOK.Render(s) }
func URLS(s string) string    { return URL.Render(s) }

// HorizontalDivider returns a line of dashes.
func HorizontalDivider(width int) string {
	if width <= 0 {
		width = 60
	}
	return Divider.Render(strings.Repeat("─", width))
}

// Badge renders a pill badge for a service state.
func Badge(state, label string) string {
	switch state {
	case "running", "healthy", "ok", "synced":
		return BadgeOK.Render(label)
	case "error", "failed", "exited", "stopped":
		return BadgeError.Render(label)
	case "warning", "partial", "unhealthy", "restarting":
		return BadgeWarning.Render(label)
	case "info", "pending", "idle":
		return BadgeInfo.Render(label)
	default:
		return BadgeDim.Render(label)
	}
}
