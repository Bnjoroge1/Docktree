package docker

import (
	"errors"
	"testing"
)

func TestComposeCommandArgs(t *testing.T) {
	cmd := ComposeCommand{
		ProjectName: "repo-branch-abcdef",
		Files:       []string{"compose.yml", ".docktree/generated/override.yml"},
		CommandArgs: []string{"up", "-d"},
	}
	got := cmd.Args()
	want := []string{"compose", "-f", "compose.yml", "-f", ".docktree/generated/override.yml", "-p", "repo-branch-abcdef", "up", "-d"}
	if len(got) != len(want) {
		t.Fatalf("args length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q; all %#v", i, got[i], want[i], got)
		}
	}
}

func TestComposeCommandArgsProfiles(t *testing.T) {
	cmd := ComposeCommand{
		ProjectName: "repo-branch-abcdef",
		Files:       []string{"compose.yml"},
		Profiles:    []string{"seed", "debug"},
		CommandArgs: []string{"up", "-d"},
	}
	got := cmd.Args()
	want := []string{"compose", "-f", "compose.yml", "-p", "repo-branch-abcdef", "--profile", "seed", "--profile", "debug", "up", "-d"}
	if len(got) != len(want) {
		t.Fatalf("args length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q; all %#v", i, got[i], want[i], got)
		}
	}
}

func TestIsPortBindError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "address already in use", err: &CommandError{Err: errors.New("exit status 1"), Stderr: "Bind for 0.0.0.0:41000 failed: port is already allocated"}, want: true},
		{name: "bind address already in use", err: errors.New("listen tcp 127.0.0.1:41000: bind: address already in use"), want: true},
		{name: "other docker error", err: errors.New("no such service: web"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPortBindError(tt.err); got != tt.want {
				t.Fatalf("IsPortBindError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNetworkPoolError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "non overlapping pool", err: &CommandError{Err: errors.New("exit status 1"), Stderr: "could not find an available, non-overlapping IPv4 address pool among the defaults to assign to the network"}, want: true},
		{name: "fully subnetted", err: errors.New("all predefined address pools have been fully subnetted"), want: true},
		{name: "other docker error", err: errors.New("no such service: web"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNetworkPoolError(tt.err); got != tt.want {
				t.Fatalf("IsNetworkPoolError() = %v, want %v", got, tt.want)
			}
		})
	}
}
