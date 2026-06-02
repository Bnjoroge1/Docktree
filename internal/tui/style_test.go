package tui

import (
	"strings"
	"testing"
)

func TestStylesRenderWithoutPanic(t *testing.T) {
	// Verify all exported styles produce non-empty strings
	if got := Brand.Render("Docktree"); got == "" {
		t.Error("Brand style produced empty output")
	}
	if got := Accent.Render("test"); got == "" {
		t.Error("Accent style produced empty output")
	}
	if got := OK.Render("✓"); got == "" {
		t.Error("OK style produced empty output")
	}
	if got := Error.Render("✗"); got == "" {
		t.Error("Error style produced empty output")
	}
	if got := Warning.Render("⚠"); got == "" {
		t.Error("Warning style produced empty output")
	}
	if got := Text.Render("hello"); got == "" {
		t.Error("Text style produced empty output")
	}
	if got := Muted.Render("muted"); got == "" {
		t.Error("Muted style produced empty output")
	}
	if got := Dim.Render("dim"); got == "" {
		t.Error("Dim style produced empty output")
	}
	if got := URL.Render("http://example.com"); got == "" {
		t.Error("URL style produced empty output")
	}
}

func TestSprintHelpers(t *testing.T) {
	// Helpers should wrap text with ANSI codes in a TTY context
	// (lipgloss detects TTY; in tests it may be plain text)
	got := BrandS("x")
	if !strings.Contains(got, "x") {
		t.Errorf("BrandS did not contain input: %q", got)
	}
	got = OKS("ok")
	if !strings.Contains(got, "ok") {
		t.Errorf("OKS did not contain input: %q", got)
	}
	got = ErrorS("err")
	if !strings.Contains(got, "err") {
		t.Errorf("ErrorS did not contain input: %q", got)
	}
	got = AccentS("accent")
	if !strings.Contains(got, "accent") {
		t.Errorf("AccentS did not contain input: %q", got)
	}
	got = DimS("dim")
	if !strings.Contains(got, "dim") {
		t.Errorf("DimS did not contain input: %q", got)
	}
	got = MutedS("muted")
	if !strings.Contains(got, "muted") {
		t.Errorf("MutedS did not contain input: %q", got)
	}
	got = URLS("url")
	if !strings.Contains(got, "url") {
		t.Errorf("URLS did not contain input: %q", got)
	}
}

func TestPaletteColorsAreValidHex(t *testing.T) {
	colors := map[string]string{
		"ColorAccent":      ColorAccent,
		"ColorAccentLight": ColorAccentLight,
		"ColorOK":          ColorOK,
		"ColorError":       ColorError,
		"ColorWarning":     ColorWarning,
		"ColorText":        ColorText,
		"ColorTextMuted":   ColorTextMuted,
		"ColorTextDim":     ColorTextDim,
		"ColorBorder":      ColorBorder,
	}
	for name, hex := range colors {
		if len(hex) != 7 || hex[0] != '#' {
			t.Errorf("%s is not a valid 7-char hex color: %q", name, hex)
		}
	}
}
