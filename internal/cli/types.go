package cli

import (
	"encoding/json"
	"io"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
)

type commandFunc func(*Context) (any, int, error)

type Context struct {
	Args     []string
	Renderer *output.Renderer
	Stdout   io.Writer
	Stderr   io.Writer
	Steps    *tui.StepPrinter
}

type UpResult struct {
	Instance        *state.Instance    `json:"instance"`
	CreatedWorktree string             `json:"created_worktree,omitempty"`
	ComposeFiles    []string           `json:"compose_files"`
	OverrideFile    string             `json:"override_file"`
	ClearFile       string             `json:"clear_file,omitempty"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	Services        []string           `json:"services"`
	SharedServices  []string           `json:"shared_services,omitempty"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	Scaffolded      bool               `json:"scaffolded,omitempty"`
	Synced          bool               `json:"synced,omitempty"`
	AlreadyRunning  bool               `json:"already_running,omitempty"`
	StaleCopies     []string           `json:"stale_copies,omitempty"`
}

type ValidateResult struct {
	Valid           bool               `json:"valid"`
	Services        []string           `json:"services"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	Errors          []string           `json:"errors,omitempty"`
}

type DryRunResult struct {
	DryRun          bool               `json:"dry_run"`
	InstanceName    string             `json:"instance_name"`
	ComposeFiles    []string           `json:"compose_files"`
	Services        []string           `json:"services"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	OverridePreview string             `json:"override_preview,omitempty"`
	ClearPreview    string             `json:"clear_preview,omitempty"`
}

type DownResult struct {
	Instance       *state.Instance `json:"instance,omitempty"`
	AlreadyStopped bool            `json:"already_stopped,omitempty"`
	DryRun         bool            `json:"dry_run,omitempty"`
	Services       []string        `json:"services,omitempty"`
	ComposeFiles   []string        `json:"compose_files,omitempty"`
	DroppedTenants []string        `json:"dropped_tenants,omitempty"`
	DroppedVolumes []string        `json:"dropped_volumes,omitempty"`
}

type StopResult struct {
	Instance       *state.Instance `json:"instance,omitempty"`
	AlreadyStopped bool            `json:"already_stopped,omitempty"`
	DryRun         bool            `json:"dry_run,omitempty"`
	Services       []string        `json:"services,omitempty"`
	ComposeFiles   []string        `json:"compose_files,omitempty"`
}

type ComposePassthroughResult struct {
	Project      string   `json:"project"`
	ComposeFiles []string `json:"compose_files"`
	Subcommand   string   `json:"subcommand"`
	Args         []string `json:"args,omitempty"`
}

type StatusResult struct {
	Instance *state.Instance `json:"instance,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
	Text     string          `json:"text,omitempty"`
	Stopped  bool            `json:"stopped,omitempty"`
}
type StatusAllEntry struct {
	Instance      string `json:"instance"`
	Branch        string `json:"branch"`
	Running       bool   `json:"running"`
	Paused        bool   `json:"paused"`
	ServiceCount  int    `json:"service_count"`
	RunningCount  int    `json:"running_count"`
	PausedCount   int    `json:"paused_count"`
	TotalServices int    `json:"total_services"`
}

type StatusAllResult struct {
	Entries []StatusAllEntry `json:"entries"`
}

type PortsResult struct {
	Instance string       `json:"instance,omitempty"`
	All      bool         `json:"all,omitempty"`
	Entries  []PortsEntry `json:"entries,omitempty"`
}

type PortsEntry struct {
	Instance string             `json:"instance"`
	Ports    []ports.Assignment `json:"ports"`
}

type PrepareResult struct {
	RepoRoot     string   `json:"repo_root"`
	WorktreeRoot string   `json:"worktree_root"`
	Copied       []string `json:"copied,omitempty"`
	Symlinked    []string `json:"symlinked,omitempty"`
	Ran          []string `json:"ran,omitempty"`
}

type CreateResult struct {
	RepoRoot     string   `json:"repo_root"`
	WorktreeRoot string   `json:"worktree_root"`
	Branch       string   `json:"branch"`
	Copied       []string `json:"copied,omitempty"`
	Symlinked    []string `json:"symlinked,omitempty"`
	Ran          []string `json:"ran,omitempty"`
}

// helps us track stale/orphaned docktree instances
type CleanItem struct {
	Instance   string `json:"instance"`
	Reason     string `json:"reason"`
	Ports      int    `json:"ports"`
	Containers int    `json:"containers"`
	Networks   int    `json:"networks"`
	Volumes    int    `json:"volumes,omitempty"`
}

type CleanTotals struct {
	Instances  int `json:"instances"`
	Ports      int `json:"ports"`
	Containers int `json:"containers"`
	Networks   int `json:"networks"`
	Volumes    int `json:"volumes,omitempty"`
}

type CleanResult struct {
	DryRun    bool        `json:"dry_run"`
	Removed   bool        `json:"removed"`
	Volumes   bool        `json:"volumes"`
	Instances []CleanItem `json:"instances"`
	Totals    CleanTotals `json:"totals"`
}

type VolumesEntry struct {
	Instance string `json:"instance"`
	Volume   string `json:"volume"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
}

type VolumesResult struct {
	All      bool           `json:"all"`
	Instance string         `json:"instance,omitempty"`
	Entries  []VolumesEntry `json:"entries"`
}

type composePsEntry struct {
	Service    string               `json:"Service"`
	Name       string               `json:"Name"`
	State      string               `json:"State"`
	Status     string               `json:"Status"`
	Health     string               `json:"Health"`
	Image      string               `json:"Image"`
	Publishers []composePsPublisher `json:"Publishers"`
}

type composePsPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}
type SyncItem struct {
	Instance     string   `json:"instance"`
	WorktreeRoot string   `json:"worktree_root"`
	MainRoot     string   `json:"main_root"`
	Branch       string   `json:"branch"`
	Files        []string `json:"files"`
	Skipped      string   `json:"skipped,omitempty"` // reason if skipped
}

type SyncResult struct {
	Items  []SyncItem `json:"items"`
	Synced bool       `json:"synced"` // true if files were actually copied
}
