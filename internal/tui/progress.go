package tui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type StepPrinter struct {
	w     io.Writer
	lines int    // number of logical lines written
	tty   bool
	mu    sync.Mutex
	termW int // terminal width for wrap detection; 0 = no wrap tracking
}

func NewStepPrinter(w io.Writer, tty bool) *StepPrinter {
	tw := 0
	if f, ok := w.(*os.File); ok {
		tw = GetTerminalWidthFrom(f)
	}
	return &StepPrinter{w: w, tty: tty, termW: tw}
}

// linesFor returns the number of terminal rows a styled string occupies,
// accounting for terminal width wrapping. Falls back to 1 if width is unknown.
func (p *StepPrinter) linesFor(s string) int {
	if p.termW <= 0 {
		return 1
	}
	visible := lipgloss.Width(s)
	if visible <= p.termW {
		return 1
	}
	n := visible / p.termW
	if visible%p.termW != 0 {
		n++
	}
	return n
}

func (p *StepPrinter) Header(title, subtitle string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := fmt.Sprintf("%s  %s", Brand.Bold(true).Render(title), DimS(subtitle))
	fmt.Fprintln(p.w, line)
	p.lines += p.linesFor(line)
}

func (p *StepPrinter) Done(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := fmt.Sprintf("%s %s", OKS("✓"), msg)
	fmt.Fprintln(p.w, line)
	p.lines += p.linesFor(line)
}

func (p *StepPrinter) Active(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := fmt.Sprintf("%s %s", BrandS("◐"), BrandS(msg))
	fmt.Fprintln(p.w, line)
	p.lines += p.linesFor(line)
}

func (p *StepPrinter) ReplaceLast(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tty && p.lines > 0 {
		// Move cursor up by the number of actual terminal lines, then clear
		for i := 0; i < p.lines; i++ {
			fmt.Fprint(p.w, "\x1b[A")
		}
		fmt.Fprint(p.w, "\x1b[2K")
	} else {
		p.lines++
	}
	line := fmt.Sprintf("%s %s", OKS("✓"), msg)
	fmt.Fprintln(p.w, line)
	p.lines = p.linesFor(line)
}

func (p *StepPrinter) Sub(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := fmt.Sprintf("  %s", msg)
	fmt.Fprintln(p.w, line)
	p.lines += p.linesFor(line)
}

func (p *StepPrinter) SubDone(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := fmt.Sprintf("  %s %s", OKS("✓"), msg)
	fmt.Fprintln(p.w, line)
	p.lines += p.linesFor(line)
}

func (p *StepPrinter) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.tty || p.lines == 0 {
		return
	}
	for i := 0; i < p.lines; i++ {
		fmt.Fprint(p.w, "\x1b[A\x1b[2K")
	}
	fmt.Fprint(p.w, "\r")
	p.lines = 0
}

func (p *StepPrinter) Blank() {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.w)
	p.lines++
}

type SpinStep struct {
	printer *StepPrinter
	stop    chan struct{}
	done    chan struct{}
}

func (p *StepPrinter) StartSpin(msg string) *SpinStep {
	s := &SpinStep{printer: p, stop: make(chan struct{}), done: make(chan struct{})}
	go s.loop(msg)
	return s
}

func (s *SpinStep) loop(msg string) {
	defer close(s.done)
	frames := []string{"◐", "◓", "◑", "◒"}
	w := s.printer.w

	s.printer.mu.Lock()
	line := fmt.Sprintf("%s %s", Brand.Render(frames[0]), BrandS(msg))
	fmt.Fprint(w, line)
	s.printer.lines += s.printer.linesFor(line)
	s.printer.mu.Unlock()

	i := 0
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			s.printer.mu.Lock()
			if s.printer.tty {
				fmt.Fprint(w, "\x1b[2K\r")
				doneLine := fmt.Sprintf("%s %s", OKS("✓"), msg)
				fmt.Fprintln(w, doneLine)
				s.printer.lines = s.printer.linesFor(doneLine)
			} else {
				doneLine := fmt.Sprintf("%s %s", OKS("✓"), msg)
				fmt.Fprintln(w, doneLine)
				s.printer.lines += s.printer.linesFor(doneLine)
			}
			s.printer.mu.Unlock()
			return
		case <-ticker.C:
			i++
			if s.printer.tty {
				spinLine := fmt.Sprintf("\r%s %s", Brand.Render(frames[i%len(frames)]), BrandS(msg))
				fmt.Fprint(w, spinLine)
			}
		}
	}
}

func (s *SpinStep) Stop() {
	close(s.stop)
	<-s.done
}

func IsTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		stat, err := f.Stat()
		if err != nil {
			return false
		}
		return (stat.Mode() & os.ModeCharDevice) != 0
	}
	return false
}
