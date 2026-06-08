package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table renders aligned columns.  It does NOT add a border; the caller
// decides whether to wrap the returned content in tui.Box.
type Table struct {
	Headers []string
	Rows    [][]string
	// Padding between columns (default 2)
	Padding int
	// CellPad is the padding inside each cell for bordered tables (default 1)
	CellPad int
	// TermWidth is the maximum total width for the table output.
	// 0 means unlimited.
	TermWidth int
}

func (t *Table) padding() int {
	if t.Padding == 0 {
		return 2
	}
	return t.Padding
}

func (t *Table) cellPad() int {
	if t.CellPad == 0 {
		return 1
	}
	return t.CellPad
}

func (t *Table) computeWidths(vals [][]string) []int {
	colCount := len(t.Headers)
	widths := make([]int, colCount)
	for i, h := range t.Headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range vals {
		for i := 0; i < colCount && i < len(row); i++ {
			if w := lipgloss.Width(row[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// capWidths reduces column widths to fit within maxWidth, subtracting
// border/separator overhead for bordered tables. Returns the effective
// per-column overhead (borders + cellPad) so callers can account for it.
func (t *Table) capWidths(widths []int, bordered bool, maxWidth int) int {
	if maxWidth <= 0 {
		return 0
	}

	colCount := len(widths)
	cp := t.cellPad()

	// Calculate overhead per column for bordered tables:
	// left cellPad + right cellPad + │ border = cp*2 + 1
	// Plus separator │ between columns (colCount - 1 of them)
	var overhead int
	if bordered {
		overhead = colCount*(cp*2+1) + (colCount - 1)
	}

	available := maxWidth - overhead
	if available <= 0 {
		// Terminal too narrow for borders; use at least 4 chars per column
		available = colCount * 4
	}

	// Sum current widths
	total := 0
	for _, w := range widths {
		total += w
	}

	if total <= available {
		return overhead
	}

	// Reduce proportionally, but never below 3 chars per column
	// First, ensure minimum widths
	minWidth := 3
	remaining := available
	for i := range widths {
		if widths[i] < minWidth {
			remaining -= minWidth
		} else {
			remaining -= widths[i]
		}
	}

	// If we went negative, scale down from minWidth
	if remaining < 0 {
		// Give each column at least minWidth, distribute rest proportionally
		for i := range widths {
			widths[i] = minWidth
		}
		remaining = available - colCount*minWidth
		if remaining > 0 {
			// Distribute proportionally based on original relative sizes
			totalOrig := 0
			for _, w := range widths {
				totalOrig += w
			}
			if totalOrig > 0 {
				for i := range widths {
					widths[i] = minWidth + (remaining * minWidth / totalOrig)
				}
			}
		}
	} else {
		// Scale down proportionally
		scale := float64(available) / float64(total)
		for i := range widths {
			w := int(float64(widths[i]) * scale)
			if w < minWidth {
				w = minWidth
			}
			widths[i] = w
		}
	}

	return overhead
}

// truncateStyled truncates a styled string to maxVisible visible characters,
// appending "…" if truncated. It preserves ANSI escape codes.
func truncateStyled(s string, maxVisible int) string {
	visible := lipgloss.Width(s)
	if visible <= maxVisible {
		return s
	}
	if maxVisible <= 1 {
		return s[:maxVisible]
	}
	// Find the byte position that corresponds to maxVisible-1 visible chars
	// by walking the string and counting visible runes
	var b strings.Builder
	visibleCount := 0
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\x1b' {
			inEscape = true
			b.WriteByte(c)
			continue
		}
		if inEscape {
			b.WriteByte(c)
			// ANSI escape sequences end with a letter in [A-Za-z]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEscape = false
			}
			continue
		}
		if visibleCount < maxVisible-1 {
			b.WriteByte(c)
			visibleCount++
		}
	}
	b.WriteString("…")
	return b.String()
}

// Render returns the aligned table content (no border).
func (t *Table) Render() string {
	return t.RenderStyled(func(row, col int, val string) string {
		if row == -1 {
			return TableHeader.Render(val)
		}
		return val
	})
}

// RenderStyled returns the aligned table content (no border).
func (t *Table) RenderStyled(styleFn func(row, col int, val string) string) string {
	if len(t.Headers) == 0 {
		return ""
	}
	pad := strings.Repeat(" ", t.padding())
	colCount := len(t.Headers)

	// Pre-style everything so widths are accurate.
	styledHeaders := make([]string, colCount)
	for i, h := range t.Headers {
		styledHeaders[i] = styleFn(-1, i, h)
	}
	styledRows := make([][]string, len(t.Rows))
	for r, row := range t.Rows {
		styledRows[r] = make([]string, colCount)
		for i := 0; i < colCount; i++ {
			if i < len(row) {
				styledRows[r][i] = styleFn(r, i, row[i])
			}
		}
	}

	widths := t.computeWidths(append([][]string{styledHeaders}, styledRows...))
	t.capWidths(widths, false, t.TermWidth)

	var b strings.Builder
	for i := 0; i < colCount; i++ {
		if i > 0 {
			b.WriteString(pad)
		}
		cell := truncateStyled(styledHeaders[i], widths[i])
		b.WriteString(fmt.Sprintf("%-*s", widths[i], cell))
	}
	b.WriteByte('\n')
	for _, row := range styledRows {
		for i := 0; i < colCount; i++ {
			if i > 0 {
				b.WriteString(pad)
			}
			cell := truncateStyled(row[i], widths[i])
			b.WriteString(fmt.Sprintf("%-*s", widths[i], cell))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// RenderBorderedStyled returns a table with box-drawing borders and column separators.
func (t *Table) RenderBorderedStyled(styleFn func(row, col int, val string) string) string {
	if len(t.Headers) == 0 {
		return ""
	}
	colCount := len(t.Headers)
	cp := t.cellPad()
	pad := strings.Repeat(" ", cp)

	styledHeaders := make([]string, colCount)
	for i, h := range t.Headers {
		styledHeaders[i] = styleFn(-1, i, h)
	}
	styledRows := make([][]string, len(t.Rows))
	for r, row := range t.Rows {
		styledRows[r] = make([]string, colCount)
		for i := 0; i < colCount; i++ {
			if i < len(row) {
				styledRows[r][i] = styleFn(r, i, row[i])
			}
		}
	}

	widths := t.computeWidths(append([][]string{styledHeaders}, styledRows...))
	t.capWidths(widths, true, t.TermWidth)

	borderColor := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBorder))
	bc := func(s string) string { return borderColor.Render(s) }

	hLine := func(left, mid, right string) string {
		var b strings.Builder
		b.WriteString(bc(left))
		for i, w := range widths {
			b.WriteString(bc(strings.Repeat("─", w+cp*2)))
			if i < colCount-1 {
				b.WriteString(bc(mid))
			}
		}
		b.WriteString(bc(right))
		return b.String()
	}

	dataRow := func(cells []string) string {
		var b strings.Builder
		b.WriteString(bc("│"))
		for i, cell := range cells {
			w := widths[i]
			truncated := truncateStyled(cell, w)
			visible := lipgloss.Width(truncated)
			fill := w - visible
			if fill < 0 {
				fill = 0
			}
			b.WriteString(pad)
			b.WriteString(truncated)
			b.WriteString(strings.Repeat(" ", fill))
			b.WriteString(pad)
			b.WriteString(bc("│"))
		}
		return b.String()
	}

	var b strings.Builder
	b.WriteString(hLine("╭", "┬", "╮"))
	b.WriteByte('\n')
	b.WriteString(dataRow(styledHeaders))
	b.WriteByte('\n')
	b.WriteString(hLine("├", "┼", "┤"))
	b.WriteByte('\n')
	for _, row := range styledRows {
		b.WriteString(dataRow(row))
		b.WriteByte('\n')
	}
	b.WriteString(hLine("╰", "┴", "╯"))
	return b.String()
}

// RenderBordered returns a bordered table with default header styling.
func (t *Table) RenderBordered() string {
	return t.RenderBorderedStyled(func(row, col int, val string) string {
		if row == -1 {
			return TableHeader.Render(val)
		}
		return val
	})
}
