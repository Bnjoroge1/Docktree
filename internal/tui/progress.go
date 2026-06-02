package tui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type StepPrinter struct {
	w     io.Writer
	lines int
	tty   bool
	mu    sync.Mutex
}

func NewStepPrinter(w io.Writer, tty bool) *StepPrinter {
	return &StepPrinter{w: w, tty: tty}
}

func (p *StepPrinter) Header(title, subtitle string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, "%s  %s\n", Brand.Bold(true).Render(title), DimS(subtitle))
	p.lines++
}

func (p *StepPrinter) Done(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, "%s %s\n", OKS("✓"), msg)
	p.lines++
}

func (p *StepPrinter) Active(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, "%s %s\n", BrandS("◐"), BrandS(msg))
	p.lines++
}

func (p *StepPrinter) ReplaceLast(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tty && p.lines > 0 {
		fmt.Fprint(p.w, "\x1b[A\x1b[2K")
	} else {
		p.lines++
	}
	fmt.Fprintf(p.w, "%s %s\n", OKS("✓"), msg)
}

func (p *StepPrinter) Sub(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, "  %s\n", msg)
	p.lines++
}

func (p *StepPrinter) SubDone(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, "  %s %s\n", OKS("✓"), msg)
	p.lines++
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
	fmt.Fprintf(w, "%s %s", Brand.Render(frames[0]), BrandS(msg))
	s.printer.lines++
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
				fmt.Fprintf(w, "%s %s\n", OKS("✓"), msg)
			} else {
				fmt.Fprintf(w, "\n%s %s\n", OKS("✓"), msg)
				s.printer.lines++
			}
			s.printer.mu.Unlock()
			return
		case <-ticker.C:
			i++
			if s.printer.tty {
				fmt.Fprintf(w, "\r%s %s", Brand.Render(frames[i%len(frames)]), BrandS(msg))
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
