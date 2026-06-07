package cli

import (
	"sort"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/provision"
	"github.com/bnjoroge/docktree/internal/state"
)

func makePlan(services map[string]config.SharedService, platformServices map[string]composetypes.ServiceConfig) *platformPlan {
	proj := &composetypes.Project{
		Services: composetypes.Services{},
	}
	for name, svc := range platformServices {
		proj.Services[name] = svc
	}
	return &platformPlan{
		Project:         "docktree-platform-myrepo",
		RepoSlug:        "myrepo",
		PlatformProject: (*compose.PlatformComposeProject)(proj),
		Shared:          config.SharedConfig{Services: services},
	}
}

func makeInst(repoRoot, name string) *state.Instance {
	return &state.Instance{RepoRoot: repoRoot, Name: name}
}

// TestTenantBindingsForInstanceSingleDB checks that a service using the legacy
// url_envs (no databases map) produces one binding with no logical DB prefix.
func TestTenantBindingsForInstanceSingleDB(t *testing.T) {
	plan := makePlan(
		map[string]config.SharedService{
			"db": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}},
		},
		map[string]composetypes.ServiceConfig{"db": {}},
	)
	inst := makeInst("/repos/myrepo", "feat-abc123")

	bindings := tenantBindingsForInstance(plan, inst)
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	want := provision.TenantNameForDatabase("myrepo", "feat-abc123", "")
	if bindings[0].TenantDB != want {
		t.Errorf("TenantDB = %q, want %q", bindings[0].TenantDB, want)
	}
	if bindings[0].Config.TenantName != want {
		t.Errorf("Config.TenantName = %q, want %q", bindings[0].Config.TenantName, want)
	}
}

// TestTenantBindingsForInstanceMultiDB checks that a service with a databases
// map produces one binding per logical DB, each with the correct prefixed name.
func TestTenantBindingsForInstanceMultiDB(t *testing.T) {
	plan := makePlan(
		map[string]config.SharedService{
			"temporal": {
				Kind:    "postgres",
				Tenancy: "per_database",
				Databases: map[string]config.SharedDatabase{
					"main":       {URLEnvs: []string{"TEMPORAL_DB_URL"}},
					"visibility": {URLEnvs: []string{"TEMPORAL_VISIBILITY_URL"}},
				},
			},
		},
		map[string]composetypes.ServiceConfig{"temporal": {}},
	)
	inst := makeInst("/repos/myrepo", "feat-abc123")

	bindings := tenantBindingsForInstance(plan, inst)
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}

	sort.Slice(bindings, func(i, j int) bool { return bindings[i].TenantDB < bindings[j].TenantDB })

	wantMain := provision.TenantNameForDatabase("myrepo", "feat-abc123", "main")
	wantVis := provision.TenantNameForDatabase("myrepo", "feat-abc123", "visibility")

	// main comes before visibility alphabetically
	if bindings[0].TenantDB != wantMain {
		t.Errorf("binding[0].TenantDB = %q, want %q", bindings[0].TenantDB, wantMain)
	}
	if bindings[1].TenantDB != wantVis {
		t.Errorf("binding[1].TenantDB = %q, want %q", bindings[1].TenantDB, wantVis)
	}
}

// TestTenantBindingsSkipsFullShare confirms full_share services are ignored.
func TestTenantBindingsSkipsFullShare(t *testing.T) {
	plan := makePlan(
		map[string]config.SharedService{
			"redis": {Kind: "redis", Tenancy: "full_share"},
		},
		map[string]composetypes.ServiceConfig{"redis": {}},
	)
	inst := makeInst("/repos/myrepo", "feat-abc123")

	bindings := tenantBindingsForInstance(plan, inst)
	if len(bindings) != 0 {
		t.Fatalf("expected 0 bindings for full_share, got %d", len(bindings))
	}
}

// TestTenantBindingsSkipsMissingPlatformService confirms that a shared service
// with no matching platform container is silently skipped.
func TestTenantBindingsSkipsMissingPlatformService(t *testing.T) {
	plan := makePlan(
		map[string]config.SharedService{
			"db": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}},
		},
		map[string]composetypes.ServiceConfig{}, // no platform container
	)
	inst := makeInst("/repos/myrepo", "feat-abc123")

	bindings := tenantBindingsForInstance(plan, inst)
	if len(bindings) != 0 {
		t.Fatalf("expected 0 bindings when platform service missing, got %d", len(bindings))
	}
}

// TestTenantEntryLogicalDBField verifies that LogicalDB is populated correctly.
func TestTenantEntryLogicalDBField(t *testing.T) {
	entry := TenantEntry{
		Instance:  "feat-abc123",
		Service:   "temporal",
		LogicalDB: "main",
		TenantDB:  "main_myrepo_feat_abc123",
		Exists:    true,
	}
	if entry.LogicalDB != "main" {
		t.Errorf("LogicalDB = %q, want %q", entry.LogicalDB, "main")
	}
	// Empty LogicalDB means legacy single-db (displayed as "default" in the UI).
	empty := TenantEntry{Service: "db", TenantDB: "myrepo_feat_abc123"}
	if empty.LogicalDB != "" {
		t.Errorf("expected empty LogicalDB for single-db entry")
	}
}
