package tui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// SimpleSpinner is a one-line spinner that overwrites the same line with \r.
// It writes to stderr by default so it never interleaves with program stdout.
type SimpleSpinner struct {
	title string
	w     io.Writer
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

// Start begins the spinner. No-op if already started.
func (s *SimpleSpinner) Start(title string) {
	s.once.Do(func() {
		s.title = title
		if s.w == nil {
			s.w = os.Stderr
		}
		s.stop = make(chan struct{})
		s.done = make(chan struct{})
		go s.loop()
	})
}

func (s *SimpleSpinner) loop() {
	defer close(s.done)
	frames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	i := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	fmt.Fprintf(s.w, "%s %s", frames[0], s.title)

	for {
		select {
		case <-s.stop:
			// Clear entire line and return cursor to start (ANSI EL2 + CR).
			fmt.Fprint(s.w, "\x1b[2K\r")
			return
		case <-ticker.C:
			i++
			fmt.Fprintf(s.w, "\r%s %s", frames[i%len(frames)], s.title)
		}
	}
}

// Stop halts the spinner and clears its line.
func (s *SimpleSpinner) Stop() {
	s.once.Do(func() {}) // ensure channels exist even if Start was never called
	if s.stop != nil {
		close(s.stop)
		<-s.done
	}
}

// RunWithSpinner runs fn while showing a spinner on stderr.
// stdout is suppressed during fn to avoid interleaving.
func RunWithSpinner(stdout io.Writer, title string, fn func() error) error {
	spinner := &SimpleSpinner{w: os.Stderr}
	spinner.Start(title)
	err := fn()
	spinner.Stop()
	return err
}
