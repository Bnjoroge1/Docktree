package tui

import (
	"strings"
	"testing"
)

func TestTableRenderBasic(t *testing.T) {
	tbl := Table{
		Headers: []string{"NAME", "VALUE"},
		Rows: [][]string{
			{"foo", "123"},
			{"barbaz", "45"},
		},
		Padding: 2,
	}
	out := tbl.Render()
	if out == "" {
		t.Fatal("expected non-empty table output")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Plain content: header + 2 rows = 3 lines
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "NAME") || !strings.Contains(lines[0], "VALUE") {
		t.Errorf("header missing expected text: %q", lines[0])
	}
	if !strings.Contains(lines[1], "foo") {
		t.Errorf("row 1 missing foo: %q", lines[1])
	}
	if !strings.Contains(lines[2], "barbaz") {
		t.Errorf("row 2 missing barbaz: %q", lines[2])
	}
}

func TestTableRenderStyled(t *testing.T) {
	tbl := Table{
		Headers: []string{"A", "B"},
		Rows: [][]string{
			{"x", "y"},
		},
		Padding: 1,
	}
	out := tbl.RenderStyled(func(row, col int, val string) string {
		if row == -1 {
			return "[" + val + "]"
		}
		return "(" + val + ")"
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "[A]") {
		t.Errorf("expected styled header, got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "(x)") {
		t.Errorf("expected styled row, got: %q", lines[1])
	}
}

func TestTableNoHeaders(t *testing.T) {
	tbl := Table{Rows: [][]string{{"a", "b"}}}
	if got := tbl.Render(); got != "" {
		t.Errorf("expected empty render for table with no headers, got: %q", got)
	}
}
