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
	Instance            *state.Instance    `json:"instance"`
	CreatedWorktree     string             `json:"created_worktree,omitempty"`
	ComposeFiles        []string           `json:"compose_files"`
	OverrideFile        string             `json:"override_file"`
	ClearFile           string             `json:"clear_file,omitempty"`
	Ports               []ports.Assignment `json:"ports,omitempty"`
	Services            []string           `json:"services"`
	SharedServices      []string           `json:"shared_services,omitempty"`
	IsolatedVolumes     []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings         []compose.Warning  `json:"env_warnings,omitempty"`
	Scaffolded          bool               `json:"scaffolded,omitempty"`
	Synced              bool               `json:"synced,omitempty"`
	AlreadyRunning      bool               `json:"already_running,omitempty"`
	StaleCopies         []string           `json:"stale_copies,omitempty"`
	Hint                string             `json:"hint,omitempty"`
	Profiles            []string           `json:"profiles,omitempty"`
	SkippedServices     []string           `json:"skipped_services,omitempty"`
	DroppedDependencies []string           `json:"dropped_dependencies,omitempty"`
	SavedSkippedServices []string          `json:"saved_skipped_services,omitempty"`
	SkipClearApplied     bool               `json:"skip_clear_applied,omitempty"`
}

type ValidateResult struct {
	Valid           bool               `json:"valid"`
	Services        []string           `json:"services"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	Errors          []string           `json:"errors,omitempty"`
	Profiles            []string `json:"profiles,omitempty"`
	SkippedServices     []string `json:"skipped_services,omitempty"`
	DroppedDependencies []string `json:"dropped_dependencies,omitempty"`
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
	Profiles            []string `json:"profiles,omitempty"`
	SkippedServices     []string `json:"skipped_services,omitempty"`
	DroppedDependencies []string `json:"dropped_dependencies,omitempty"`
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

// LsEntry is one row from docker compose ls --format json.
type LsEntry struct {
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	ConfigFiles string `json:"ConfigFiles"`
}

// LsResult renders docker compose ls with docktree table formatting.
type LsResult struct {
	Entries []LsEntry `json:"entries"`
}

// ImagesEntry is one row from docker compose images --format json.
type ImagesEntry struct {
	ID            string `json:"ID"`
	ContainerName string `json:"ContainerName"`
	Repository    string `json:"Repository"`
	Tag           string `json:"Tag"`
	Platform      string `json:"Platform,omitempty"`
	Size          int64  `json:"Size"`
	Created       string `json:"LastTagTime,omitempty"`
}

// ImagesResult renders docker compose images with docktree table formatting.
type ImagesResult struct {
	ProjectName string        `json:"project_name,omitempty"`
	Entries []ImagesEntry `json:"entries"`
}

// TopRow is one row from docker compose top (parsed from text).
type TopRow struct {
	Service string `json:"service"`
	Num     string `json:"num"`
	UID     string `json:"uid"`
	PID     string `json:"pid"`
	PPID    string `json:"ppid"`
	CPU     string `json:"cpu"`
	STime   string `json:"stime"`
	TTY     string `json:"tty"`
	Time    string `json:"time"`
	Cmd     string `json:"cmd"`
}

// TopResult renders docker compose top with docktree table formatting.
type TopResult struct {
	Rows []TopRow `json:"rows"`
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
	ProxyURL      string `json:"proxy_url,omitempty"`
	TunnelURL     string `json:"tunnel_url,omitempty"`
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

// InitTodo describes one decision point in a generated docktree.yml.
// The agent skill uses these to ask the user targeted questions.
type InitTodo struct {
	Path     string   `json:"path"`              // YAML path, e.g. "shared.services.db.tenancy"
	Question string   `json:"question"`          // Human-readable question
	Kind     string   `json:"kind"`              // "enum", "string_list", "string"
	Options  []string `json:"options,omitempty"` // Allowed values when Kind == "enum"
}

// InitWarning surfaces a non-blocking concern the agent should show the user.
type InitWarning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// InitResult is the --json output of `docktree init`.
type InitResult struct {
	Written  string        `json:"written"`         // path to proposed or final config
	Todos    []InitTodo    `json:"todos,omitempty"` // decisions the agent should walk through
	Warnings []InitWarning `json:"warnings,omitempty"`
}
