package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/setup"
	"gopkg.in/yaml.v3"
)

func TestParseInitOptions(t *testing.T) {
	opts, err := parseInitOptions([]string{"--dry-run", "--force"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.dryRun || !opts.force {
		t.Errorf("dryRun=%v force=%v, want both true", opts.dryRun, opts.force)
	}
	opts, err = parseInitOptions([]string{"--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.help {
		t.Errorf("help=%v, want true", opts.help)
	}
	_, err = parseInitOptions([]string{"--bogus"})
	if err == nil {
		t.Errorf("expected error for unknown flag")
	}
}

func TestBuildProposal(t *testing.T) {
	files := []string{"docker-compose.yml", "docker-compose.override.yml"}
	envFiles := []string{".env"}
	symlinkDirs := []string{"node_modules"}
	candidates := []setup.ServiceCandidate{
		{ServiceName: "db", Kind: "postgres", Image: "postgres:16", URLEnvs: []string{"DATABASE_URL"}},
		{ServiceName: "cache", Kind: "redis", Image: "redis:7"},
	}
	proposal := buildProposal(files, envFiles, symlinkDirs, candidates)
	if !strings.Contains(proposal, "compose:") {
		t.Error("missing compose section")
	}
	if !strings.Contains(proposal, "- docker-compose.yml") {
		t.Error("missing compose file entry")
	}
	if !strings.Contains(proposal, "copy:") {
		t.Error("missing setup.copy")
	}
	if !strings.Contains(proposal, "- .env") {
		t.Error("missing .env in copy")
	}
	if !strings.Contains(proposal, "symlink:") {
		t.Error("missing setup.symlink")
	}
	if !strings.Contains(proposal, "- node_modules") {
		t.Error("missing node_modules in symlink")
	}
	if !strings.Contains(proposal, "shared:") {
		t.Error("missing shared section")
	}
	if !strings.Contains(proposal, "kind: postgres") {
		t.Error("missing postgres kind")
	}
	if !strings.Contains(proposal, "kind: redis") {
		t.Error("missing redis kind")
	}
	if !strings.Contains(proposal, "# TODO: choose tenancy mode") {
		t.Error("missing tenancy TODO")
	}
	if !strings.Contains(proposal, "url_envs:") {
		t.Error("missing url_envs for db")
	}
	if !strings.Contains(proposal, "- DATABASE_URL") {
		t.Error("missing DATABASE_URL entry")
	}
	// cache (redis) should NOT have url_envs since it had none.
	cacheIdx := strings.Index(proposal, "cache:")
	dbIdx := strings.Index(proposal, "db:")
	if cacheIdx < dbIdx {
		// cache comes before db; check that between cache and db there's no url_envs
		between := proposal[cacheIdx:dbIdx]
		if strings.Contains(between, "url_envs:") {
			t.Error("cache should not have url_envs")
		}
	}
}

func TestBuildProposalNoCandidates(t *testing.T) {
	proposal := buildProposal([]string{"compose.yml"}, nil, nil, nil)
	if strings.Contains(proposal, "shared:") {
		t.Error("shared section should be absent when no candidates")
	}
	if !strings.Contains(proposal, "# copy:") {
		t.Error("copy should be commented out when no envFiles")
	}
	if !strings.Contains(proposal, "# symlink:") {
		t.Error("symlink should be commented out when no symlinkDirs")
	}
}

func TestBuildTodos(t *testing.T) {
	candidates := []setup.ServiceCandidate{
		{ServiceName: "db", Kind: "postgres", Image: "postgres:16", URLEnvs: []string{"DATABASE_URL", "DB_DSN"}},
		{ServiceName: "redis", Kind: "redis", Image: "redis:7"},
	}
	todos := buildTodos(candidates)
	if len(todos) != 3 { // db tenancy + db url_envs + redis tenancy
		t.Fatalf("expected 3 todos, got %d", len(todos))
	}
	if todos[0].Path != "shared.services.db.tenancy" {
		t.Errorf("todo[0].path = %q", todos[0].Path)
	}
	if todos[0].Kind != "enum" {
		t.Errorf("todo[0].kind = %q", todos[0].Kind)
	}
	if len(todos[0].Options) != 2 { // postgres supports per_database and full_share
		t.Errorf("todo[0].options = %v", todos[0].Options)
	}
	if todos[1].Path != "shared.services.db.url_envs" {
		t.Errorf("todo[1].path = %q", todos[1].Path)
	}
	if todos[1].Kind != "string_list" {
		t.Errorf("todo[1].kind = %q", todos[1].Kind)
	}
	if todos[2].Path != "shared.services.redis.tenancy" {
		t.Errorf("todo[2].path = %q", todos[2].Path)
	}
	if len(todos[2].Options) != 1 { // redis only supports full_share
		t.Errorf("todo[2].options = %v", todos[2].Options)
	}
}

func TestTenancyOptions(t *testing.T) {
	if got := tenancyOptions("postgres"); len(got) != 2 {
		t.Errorf("postgres options = %v", got)
	}
	if got := tenancyOptions("redis"); len(got) != 1 || got[0] != "full_share" {
		t.Errorf("redis options = %v", got)
	}
	if got := tenancyOptions("s3"); len(got) != 1 || got[0] != "full_share" {
		t.Errorf("s3 options = %v", got)
	}
}

func TestReadAnswers(t *testing.T) {
	r := strings.NewReader(`{"shared.services.db.tenancy":"per_database","shared.services.db.url_envs":["DATABASE_URL"]}`)
	answers, err := readAnswers(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answers["shared.services.db.tenancy"] != "per_database" {
		t.Errorf("tenancy = %v", answers["shared.services.db.tenancy"])
	}
	arr, ok := answers["shared.services.db.url_envs"].([]any)
	if !ok || len(arr) != 1 || arr[0] != "DATABASE_URL" {
		t.Errorf("url_envs = %v", answers["shared.services.db.url_envs"])
	}
}

func TestReadAnswersEmpty(t *testing.T) {
	answers, err := readAnswers(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(answers) != 0 {
		t.Errorf("expected empty, got %v", answers)
	}
}

func TestRunInitApplyBuildsConfig(t *testing.T) {
	// Test the config-building logic that --apply uses: build a config
	// from deterministic data + user answers, validate, and marshal.
	cfg := config.Defaults()
	cfg.Compose.Files = []string{"docker-compose.yml"}
	cfg.Setup.Copy = []string{".env"}
	cfg.Setup.Symlink = []string{"node_modules"}
	cfg.Shared.Services = map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}},
	}

	if err := config.ValidateShared(cfg.Shared, cfg.Volumes.Share); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "tenancy: per_database") {
		t.Errorf("marshalled config missing tenancy:\n%s", s)
	}
	if !strings.Contains(s, "- DATABASE_URL") {
		t.Errorf("marshalled config missing url_envs:\n%s", s)
	}
	if !strings.Contains(s, "- docker-compose.yml") {
		t.Errorf("marshalled config missing compose files:\n%s", s)
	}
	if !strings.Contains(s, "- .env") {
		t.Errorf("marshalled config missing copy:\n%s", s)
	}
	if !strings.Contains(s, "- node_modules") {
		t.Errorf("marshalled config missing symlink:\n%s", s)
	}
}

func TestInitResultJSONRendering(t *testing.T) {
	var buf bytes.Buffer
	output.New(&buf, true).Render(InitResult{
		Written: "/repo/docktree.yml.proposed",
		Todos: []InitTodo{
			{Path: "shared.services.db.tenancy", Question: "q", Kind: "enum", Options: []string{"full_share", "per_database"}},
		},
		Warnings: []InitWarning{
			{Path: "shared.services.db", Message: "secrets wrapper detected"},
		},
	}, humanRenderer())
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json decode failed: %v\n%s", err, buf.String())
	}
	if got["written"] != "/repo/docktree.yml.proposed" {
		t.Errorf("written = %v", got["written"])
	}
	todos := got["todos"].([]any)
	if len(todos) != 1 {
		t.Errorf("todos len = %d", len(todos))
	}
	warnings := got["warnings"].([]any)
	if len(warnings) != 1 {
		t.Errorf("warnings len = %d", len(warnings))
	}
}

func TestDetectSecretsWrapper(t *testing.T) {
	project := &composetypes.Project{
		Services: composetypes.Services{
			"api": {Name: "api", Command: composetypes.ShellCommand{"infisical", "run", "--", "npm", "start"}},
			"db":  {Name: "db", Image: "postgres:16"},
		},
	}
	if !detectSecretsWrapper(project, "db") {
		t.Error("expected detection: api uses infisical in command")
	}
	// db is the candidate and skipped, but api still triggers.
	if !detectSecretsWrapper(project, "db") {
		t.Error("expected detection even when candidate is db")
	}
	clean := &composetypes.Project{
		Services: composetypes.Services{
			"api": {Name: "api", Entrypoint: composetypes.ShellCommand{"node", "server.js"}},
		},
	}
	if detectSecretsWrapper(clean, "db") {
		t.Error("no wrapper in command/entrypoint, should be false")
	}
	if detectSecretsWrapper(nil, "db") {
		t.Error("nil project should be false")
	}
}
