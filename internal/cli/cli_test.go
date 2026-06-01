package cli

import (
	"path/filepath"
	"testing"

	"github.com/bnjoroge/docktree/internal/config"
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
