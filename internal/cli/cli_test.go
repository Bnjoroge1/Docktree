package cli

import (
	"path/filepath"
	"testing"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	dockgit "github.com/bnjoroge/docktree/internal/git"
)

func TestParseComposeRunState(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    composeRunState
		wantErr bool
	}{
		{
			name: "running entry",
			out:  `[{"State":"running"}]`,
			want: composeRunRunning,
		},
		{
			name: "stopped entry",
			out:  `[{"State":"exited"}]`,
			want: composeRunStopped,
		},
		{
			name:    "invalid json",
			out:     `not json`,
			want:    composeRunUnknown,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseComposeRunState(tt.out)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if got != tt.want {
					t.Fatalf("state = %v, want %v", got, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("state = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorktreePathDefaultRoot(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "tmp", "Docktree")
	cfg := config.Defaults()
	got, err := worktreePath(repoRoot, &cfg, "feature/auth")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(string(filepath.Separator), "tmp", "Docktree.worktrees", "feature-auth")
	if got != want {
		t.Fatalf("worktreePath = %q, want %q", got, want)
	}
}

func TestWorktreePathConfigurableRoot(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "tmp", "Docktree")
	cfg := config.Defaults()
	cfg.Worktrees.Root = "./worktrees/${branch_slug}"
	got, err := worktreePath(repoRoot, &cfg, "feature/auth")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(string(filepath.Separator), "tmp", "Docktree", "worktrees", "feature-auth")
	if got != want {
		t.Fatalf("worktreePath = %q, want %q", got, want)
	}
}

func TestSlugWorktreeBranch(t *testing.T) {
	if got := slugWorktreeBranch("feature/auth"); got != "feature-auth" {
		t.Fatalf("slugWorktreeBranch = %q", got)
	}
	if got := slugWorktreeBranch("  FIX Bug/123  "); got != "fix-bug-123" {
		t.Fatalf("slugWorktreeBranch normalized = %q", got)
	}
}

func TestParseUpOptionsValidate(t *testing.T) {
	opts, err := parseUpOptions([]string{"--validate"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.validate {
		t.Fatal("expected validate=true")
	}
	if opts.sync || opts.help || opts.create != "" || opts.file != "" {
		t.Fatal("expected other flags to be zero")
	}
}

func TestParseUpOptionsValidateWithSync(t *testing.T) {
	opts, err := parseUpOptions([]string{"--validate", "--sync"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.validate {
		t.Fatal("expected validate=true")
	}
	if !opts.sync {
		t.Fatal("expected sync=true")
	}
}

func TestRunValidateNoServices(t *testing.T) {
	project := &compose.ComposeProject{Services: map[string]compose.Service{}}
	cfg := config.Defaults()
	repo := dockgit.RepoInfo{RepoRoot: "/tmp/Docktree", WorktreeRoot: "/tmp/Docktree", Branch: "main"}
	result, code, err := runValidate(project, nil, &cfg, repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	vr := result.(ValidateResult)
	if vr.Valid {
		t.Fatal("expected valid=false for no services")
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if len(vr.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
}

func TestRunValidateWithServices(t *testing.T) {
	project := &compose.ComposeProject{
		Services: map[string]compose.Service{
			"web": {Image: "nginx"},
		},
	}
	cfg := config.Defaults()
	repo := dockgit.RepoInfo{RepoRoot: "/tmp/Docktree", WorktreeRoot: "/tmp/Docktree", Branch: "main"}
	result, code, err := runValidate(project, nil, &cfg, repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	vr := result.(ValidateResult)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if len(vr.Services) != 1 || vr.Services[0] != "web" {
		t.Fatalf("expected services [web], got %v", vr.Services)
	}
}

func TestParseUpOptionsDryRun(t *testing.T) {
	opts, err := parseUpOptions([]string{"--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.dryRun {
		t.Fatal("expected dryRun=true")
	}
	if opts.validate || opts.sync || opts.help || opts.create != "" || opts.file != "" {
		t.Fatal("expected other flags to be zero")
	}
}

func TestParseUpOptionsDryRunValidateNotMutuallyExclusiveInParser(t *testing.T) {
	opts, err := parseUpOptions([]string{"--dry-run", "--validate"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.dryRun || !opts.validate {
		t.Fatal("expected both dryRun and validate to be set")
	}
}

func TestParsePortsOptionsAll(t *testing.T) {
	opts, err := parsePortsOptions([]string{"--all"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.all {
		t.Fatal("expected all=true")
	}
	if opts.help {
		t.Fatal("expected help=false")
	}
}

func TestParsePortsOptionsShortAll(t *testing.T) {
	opts, err := parsePortsOptions([]string{"-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.all {
		t.Fatal("expected all=true")
	}
}

func TestParsePortsOptionsHelp(t *testing.T) {
	opts, err := parsePortsOptions([]string{"-h"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.help {
		t.Fatal("expected help=true")
	}
}

func TestParsePortsOptionsUnknown(t *testing.T) {
	_, err := parsePortsOptions([]string{"--unknown"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestParseDownOptionsHelp(t *testing.T) {
	opts, err := parseDownOptions([]string{"-h"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.help {
		t.Fatal("expected help=true")
	}
}

func TestParseDownOptionsDryRun(t *testing.T) {
	opts, err := parseDownOptions([]string{"--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.dryRun {
		t.Fatal("expected dryRun=true")
	}
}

func TestParseDownOptionsEmpty(t *testing.T) {
	opts, err := parseDownOptions([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.dryRun || opts.help {
		t.Fatal("expected all flags false")
	}
	if len(opts.services) != 0 {
		t.Fatalf("expected no services, got %v", opts.services)
	}
}

func TestParseDownOptionsUnknownFlag(t *testing.T) {
	_, err := parseDownOptions([]string{"--unknown"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestParseDownOptionsServices(t *testing.T) {
	opts, err := parseDownOptions([]string{"web", "api"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.dryRun || opts.help {
		t.Fatal("expected dryRun=false, help=false")
	}
	if len(opts.services) != 2 || opts.services[0] != "web" || opts.services[1] != "api" {
		t.Fatalf("expected services [web, api], got %v", opts.services)
	}
}

func TestParseDownOptionsDryRunWithServices(t *testing.T) {
	opts, err := parseDownOptions([]string{"--dry-run", "db", "redis"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.dryRun {
		t.Fatal("expected dryRun=true")
	}
	if len(opts.services) != 2 || opts.services[0] != "db" || opts.services[1] != "redis" {
		t.Fatalf("expected services [db, redis], got %v", opts.services)
	}
}
