package tui

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// GetTerminalWidth returns the width of the terminal attached to stdout,
// or 0 if it cannot be determined.
func GetTerminalWidth() int {
	return getTermWidth(os.Stdout)
}

// GetTerminalWidthFrom returns the width of the terminal for the given file,
// or 0 if it cannot be determined.
func GetTerminalWidthFrom(f *os.File) int {
	return getTermWidth(f)
}

func getTermWidth(f *os.File) int {
	if f == nil {
		return 0
	}
	w, _, err := term.GetSize(uintptr(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}
