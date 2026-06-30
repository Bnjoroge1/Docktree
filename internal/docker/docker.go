package docker

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
)

// ComposeCommand is the command shape Docktree will execute for Docker Compose.
type ComposeCommand struct {
	ProjectName string
	Files       []string
	Profiles    []string
	CommandArgs []string
}

// Args renders the docker compose arguments without the leading "docker".
func (cmd ComposeCommand) Args() []string {
	args := []string{"compose"}
	for _, file := range cmd.Files {
		args = append(args, "-f", file)
	}
	if cmd.ProjectName != "" {
		args = append(args, "-p", cmd.ProjectName)
	}
	for _, profile := range cmd.Profiles {
		args = append(args, "--profile", profile)
	}
	args = append(args, cmd.CommandArgs...)
	return args
}

func Run(cmd ComposeCommand, stdout, stderr io.Writer) error {
	execCmd := exec.Command("docker", cmd.Args()...)
	var stderrBuf bytes.Buffer
	execCmd.Stdout = stdout
	if stderr != nil {
		execCmd.Stderr = io.MultiWriter(stderr, &stderrBuf)
	} else {
		execCmd.Stderr = &stderrBuf
	}
	if err := execCmd.Run(); err != nil {
		if stderrBuf.Len() > 0 {
			return &CommandError{Err: err, Stderr: stderrBuf.String()}
		}
		return err
	}
	return nil
}

func RunCapture(cmd ComposeCommand) (string, error) {
	execCmd := exec.Command("docker", cmd.Args()...)
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	if err := execCmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return stdout.String(), &CommandError{Err: err, Stderr: stderr.String()}
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func IsComposeAvailable() error {
	return exec.Command("docker", "compose", "version").Run()
}

func IsPortBindError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "bind") && (strings.Contains(msg, "address already in use") || strings.Contains(msg, "port is already allocated"))
}

type CommandError struct {
	Err    error
	Stderr string
}

func (e *CommandError) Error() string {
	if e.Stderr == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + ": " + e.Stderr
}

func (e *CommandError) Unwrap() error {
	return e.Err
}
