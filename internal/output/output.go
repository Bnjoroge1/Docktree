package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	ExitOK = iota
	ExitGeneral
	ExitUsage
	ExitConfig
	ExitDocker
	ExitNoop
	ExitConflict
)

type Renderer struct {
	JSON   bool
	IsTTY  bool
	Writer io.Writer
}

type ErrorPayload struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func New(stdout io.Writer, jsonMode bool) *Renderer {
	return &Renderer{JSON: jsonMode, IsTTY: IsTerminal(os.Stdin), Writer: stdout}
}

//render diff things if user is a tty or non-interactve such as an agent(support json)
func (r *Renderer) Render(data any, humanFn func(io.Writer, any)) {
	if r.Writer == nil {
		r.Writer = io.Discard
	}
	if r.JSON {
		_ = json.NewEncoder(r.Writer).Encode(data)
		return
	}
	humanFn(r.Writer, data)
}

func (r *Renderer) Error(code string, message string, details any) {
	payload := ErrorPayload{Error: code, Message: message, Details: details}
	if r.JSON || !r.IsTTY {
		_ = json.NewEncoder(r.Writer).Encode(payload)
		return
	}
	fmt.Fprintf(r.Writer, "Error [%s]: %s\n", code, message)
}

//we check if term is a char device
func IsTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
