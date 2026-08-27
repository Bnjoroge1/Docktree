//go:build integration

package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnjoroge/docktree/internal/output"
)

// monorepoJSON captures the subproject-relevant slice of UpResult/StatusResult.
type monorepoJSON struct {
	Instance struct {
		ProjectName    string `json:"project_name"`
		Subpath        string `json:"subpath"`
		StateDirectory string `json:"state_directory"`
		WorktreeRoot   string `json:"worktree_root"`
	} `json:"instance"`
	ComposeFiles []string `json:"compose_files"`
	DryRun       bool     `json:"dry_run"`
	Instances    []string `json:"instances"`
}

// setupMonorepo builds the layout from issue #61: two independent subprojects,
// each with its own docktree.yml and compose file, plus a nested package inside
// the first one.
func setupMonorepo(t *testing.T, root string) string {
	t.Helper()
	sourceCompose, err := filepath.Abs(filepath.Join("..", "testdata", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	for _, project := range []string{"project-a", "project-b"} {
		if err := os.MkdirAll(filepath.Join(repo, project, "packages", "pkg1"), 0o755); err != nil {
			t.Fatal(err)
		}
		copyFile(t, sourceCompose, filepath.Join(repo, project, "compose.yml"))
		cfg := "compose:\n  files:\n    - compose.yml\n"
		if err := os.WriteFile(filepath.Join(repo, project, "docktree.yml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "docktree@example.test")
	run(t, repo, "git", "config", "user.name", "Docktree Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")
	return repo
}

func fakeDockerEnv(t *testing.T, root string) {
	t.Helper()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeDocker(t, filepath.Join(fakeBin, "docker"), filepath.Join(root, "docker-state"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
}

func upJSONIn(t *testing.T, dir string, args ...string) monorepoJSON {
	t.Helper()
	t.Chdir(dir)
	code, stdout, errText := runCLI(append(args, "--json")...)
	if code != output.ExitOK || errText != "" {
		t.Fatalf("%v in %s: code=%d err=%s out=%s", args, dir, code, errText, stdout)
	}
	var parsed monorepoJSON
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("%v in %s: output not json: %v\n%s", args, dir, err, stdout)
	}
	return parsed
}

// TestMonorepoSubprojectsRunSideBySide is the acceptance test for issue #61:
// two subprojects in one git worktree get independent Compose identities,
// state directories, and compose files, and nested directories select their
// containing project.
func TestMonorepoSubprojectsRunSideBySide(t *testing.T) {
	root := t.TempDir()
	repo := setupMonorepo(t, root)
	fakeDockerEnv(t, root)

	a := upJSONIn(t, filepath.Join(repo, "project-a"), "up")
	// project-b comes up from a nested package directory, which must still
	// select project-b's config.
	b := upJSONIn(t, filepath.Join(repo, "project-b", "packages", "pkg1"), "up")

	if a.Instance.Subpath != "project-a" {
		t.Fatalf("project-a subpath = %q", a.Instance.Subpath)
	}
	if b.Instance.Subpath != "project-b" {
		t.Fatalf("project-b subpath = %q", b.Instance.Subpath)
	}
	if a.Instance.ProjectName == b.Instance.ProjectName {
		t.Fatalf("subprojects share Compose identity %q", a.Instance.ProjectName)
	}
	if a.Instance.WorktreeRoot != b.Instance.WorktreeRoot {
		t.Fatalf("expected one shared worktree root, got %q and %q", a.Instance.WorktreeRoot, b.Instance.WorktreeRoot)
	}
	// git reports the worktree root with symlinks resolved (macOS /private/var),
	// so expectations are anchored on the reported root rather than t.TempDir().
	resolved := a.Instance.WorktreeRoot
	for _, tc := range []struct {
		name string
		got  monorepoJSON
		sub  string
	}{{"project-a", a, "project-a"}, {"project-b", b, "project-b"}} {
		wantState := filepath.Join(resolved, tc.sub, ".docktree")
		if tc.got.Instance.StateDirectory != wantState {
			t.Fatalf("%s state dir = %q, want %q", tc.name, tc.got.Instance.StateDirectory, wantState)
		}
		if _, err := os.Stat(filepath.Join(wantState, "generated")); err != nil {
			t.Fatalf("%s generated dir missing: %v", tc.name, err)
		}
		wantCompose := filepath.Join(resolved, tc.sub, "compose.yml")
		if len(tc.got.ComposeFiles) != 1 || tc.got.ComposeFiles[0] != wantCompose {
			t.Fatalf("%s compose files = %v, want [%s]", tc.name, tc.got.ComposeFiles, wantCompose)
		}
	}

	// One .gitignore at the worktree root covers both projects.
	ignore := readFile(t, filepath.Join(resolved, ".gitignore"))
	for _, want := range []string{"project-a/.docktree/", "project-b/.docktree/"} {
		if !strings.Contains(ignore, want) {
			t.Fatalf(".gitignore missing %q:\n%s", want, ignore)
		}
	}

	// Both projects are simultaneously up in the same worktree.
	for _, dir := range []string{filepath.Join(repo, "project-a"), filepath.Join(repo, "project-b")} {
		got := upJSONIn(t, dir, "status")
		if got.Instance.ProjectName == "" {
			t.Fatalf("status in %s reported no instance", dir)
		}
	}

	// Ports are allocated per project and never collide.
	portsFor := func(dir string) []int {
		t.Helper()
		t.Chdir(dir)
		code, stdout, errText := runCLI("ports", "--json")
		if code != output.ExitOK || errText != "" {
			t.Fatalf("ports in %s: code=%d err=%s", dir, code, errText)
		}
		var result struct {
			Entries []struct {
				Ports []struct {
					HostPort int `json:"host_port"`
				} `json:"ports"`
			} `json:"entries"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatal(err)
		}
		var out []int
		for _, e := range result.Entries {
			for _, p := range e.Ports {
				out = append(out, p.HostPort)
			}
		}
		return out
	}
	aPorts := portsFor(filepath.Join(repo, "project-a"))
	bPorts := portsFor(filepath.Join(repo, "project-b"))
	if len(aPorts) == 0 || len(bPorts) == 0 {
		t.Fatalf("expected ports for both projects: a=%v b=%v", aPorts, bPorts)
	}
	for _, ap := range aPorts {
		for _, bp := range bPorts {
			if ap == bp {
				t.Fatalf("port %d allocated to both projects", ap)
			}
		}
	}

	// Explicit --config selects a project from anywhere in the repo.
	viaFlag := upJSONIn(t, repo, "--config", filepath.Join("project-b", "docktree.yml"), "status")
	if viaFlag.Instance.ProjectName != b.Instance.ProjectName {
		t.Fatalf("--config selected %q, want %q", viaFlag.Instance.ProjectName, b.Instance.ProjectName)
	}

	// down is scoped: stopping project-a leaves project-b running.
	t.Chdir(filepath.Join(repo, "project-a"))
	if code, _, errText := runCLI("down"); code != output.ExitOK {
		t.Fatalf("down project-a: code=%d err=%s", code, errText)
	}
	t.Chdir(filepath.Join(repo, "project-b"))
	if code, stdout, _ := runCLI("status", "--json"); code != output.ExitOK {
		t.Fatalf("project-b status after project-a down: code=%d out=%s", code, stdout)
	}
}

// TestMonorepoDownAllScoping covers --all vs --all-projects across multiple
// worktrees of the same subproject.
func TestMonorepoDownAllScoping(t *testing.T) {
	root := t.TempDir()
	repo := setupMonorepo(t, root)
	fakeDockerEnv(t, root)

	worktree := filepath.Join(root, "wt-feat")
	run(t, repo, "git", "worktree", "add", "-b", "feat", worktree)

	aMain := upJSONIn(t, filepath.Join(repo, "project-a"), "up")
	bMain := upJSONIn(t, filepath.Join(repo, "project-b"), "up")
	aFeat := upJSONIn(t, filepath.Join(worktree, "project-a"), "up")

	names := map[string]bool{}
	for _, n := range []string{aMain.Instance.ProjectName, bMain.Instance.ProjectName, aFeat.Instance.ProjectName} {
		if names[n] {
			t.Fatalf("duplicate identity %q across three projects", n)
		}
		names[n] = true
	}

	// --all in project-a covers both of its worktrees, not project-b.
	got := upJSONIn(t, filepath.Join(repo, "project-a"), "down", "--all", "--dry-run")
	assertSameSet(t, "down --all from project-a", got.Instances,
		[]string{aMain.Instance.ProjectName, aFeat.Instance.ProjectName})

	// --all in project-b covers only project-b.
	got = upJSONIn(t, filepath.Join(repo, "project-b"), "down", "--all", "--dry-run")
	assertSameSet(t, "down --all from project-b", got.Instances,
		[]string{bMain.Instance.ProjectName})

	// --all-projects widens to the whole repository.
	got = upJSONIn(t, filepath.Join(repo, "project-b"), "down", "--all-projects", "--dry-run")
	assertSameSet(t, "down --all-projects", got.Instances,
		[]string{aMain.Instance.ProjectName, bMain.Instance.ProjectName, aFeat.Instance.ProjectName})
}

func assertSameSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	gotSet := map[string]bool{}
	for _, g := range got {
		gotSet[g] = true
	}
	if len(gotSet) != len(want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
	for _, w := range want {
		if !gotSet[w] {
			t.Fatalf("%s: missing %q in %v", what, w, got)
		}
	}
}
