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
}

func (t *Table) padding() int {
	if t.Padding == 0 {
		return 2
	}
	return t.Padding
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

	var b strings.Builder
	for i := 0; i < colCount; i++ {
		if i > 0 {
			b.WriteString(pad)
		}
		b.WriteString(fmt.Sprintf("%-*s", widths[i], styledHeaders[i]))
	}
	b.WriteByte('\n')
	for _, row := range styledRows {
		for i := 0; i < colCount; i++ {
			if i > 0 {
				b.WriteString(pad)
			}
			b.WriteString(fmt.Sprintf("%-*s", widths[i], row[i]))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (t *Table) cellPad() int {
	if t.CellPad == 0 {
		return 1
	}
	return t.CellPad
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
			visible := lipgloss.Width(cell)
			fill := w - visible
			if fill < 0 {
				fill = 0
			}
			b.WriteString(pad)
			b.WriteString(cell)
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
