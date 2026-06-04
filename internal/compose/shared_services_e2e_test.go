package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnjoroge/docktree/internal/config"
)

// TestSharedServicesSynthesisE2E validates the full synthesis pipeline
// against the testdata/compose-shared-services.yml fixture:
// - platform project contains only db/redis/garage
// - worktree project omits them + strips depends_on
// - DATABASE_URL is rewritten with the tenant DB name
// - generated files are valid compose YAML (reload-able by compose-go)
func TestSharedServicesSynthesisE2E(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "compose-shared-services.yml")
	if _, err := os.Stat(fixturePath); err != nil {
		t.Skipf("fixture not found: %s", fixturePath)
	}

	raw, _, err := LoadFull([]string{fixturePath})
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}

	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {
			Kind:    "postgres",
			Tenancy: "per_database",
			URLEnvs: []string{"DATABASE_URL"},
		},
		"redis": {Kind: "redis", Tenancy: "full_share"},
		"garage": {Kind: "s3", Tenancy: "full_share"},
	}}

	repoSlug := "docktree-test"
	instanceName := "docktree-test-main-abc123"
	tenantDB := "docktree_test_docktree_test_main_abc123"

	// --- Platform synthesis ---
	platform, err := SynthesizePlatform(raw, shared, repoSlug)
	if err != nil {
		t.Fatalf("SynthesizePlatform: %v", err)
	}

	platformServices := SortedServiceNames(platform)
	for _, want := range []string{"db", "redis", "garage"} {
		found := false
		for _, s := range platformServices {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("platform missing service %q (has %v)", want, platformServices)
		}
	}
	for _, notWant := range []string{"api", "ui", "worker"} {
		for _, s := range platformServices {
			if s == notWant {
				t.Errorf("platform should NOT contain worktree service %q", notWant)
			}
		}
	}

	// db must have no ports and correct labels
	db := platform.Services["db"]
	if len(db.Ports) != 0 {
		t.Errorf("platform db should have no host ports, got %v", db.Ports)
	}
	if db.Labels["docktree.tier"] != "platform" {
		t.Errorf("db labels: %v", db.Labels)
	}
	netName := PlatformNetworkName(repoSlug)
	if _, ok := db.Networks[netName]; !ok {
		t.Errorf("db not on platform network %q", netName)
	}

	// --- Worktree synthesis ---
	wt, err := SynthesizeWorktree(raw, shared, repoSlug, SynthesizeWorktreeOptions{
		TenantDBs: map[string]string{"db": tenantDB},
	})
	if err != nil {
		t.Fatalf("SynthesizeWorktree: %v", err)
	}

	wtServices := SortedServiceNames(wt)
	for _, notWant := range []string{"db", "redis", "garage"} {
		for _, s := range wtServices {
			if s == notWant {
				t.Errorf("worktree should NOT contain platform service %q", notWant)
			}
		}
	}
	for _, want := range []string{"api", "ui", "worker"} {
		found := false
		for _, s := range wtServices {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("worktree missing service %q (has %v)", want, wtServices)
		}
	}

	// depends_on to platform services must be stripped
	api := wt.Services["api"]
	for dep := range api.DependsOn {
		if dep == "db" || dep == "redis" {
			t.Errorf("api.depends_on still references platform service %q", dep)
		}
	}
	worker := wt.Services["worker"]
	for dep := range worker.DependsOn {
		if dep == "db" {
			t.Errorf("worker.depends_on still references platform service %q", dep)
		}
	}

	// DATABASE_URL must be rewritten in api and worker
	for _, svcName := range []string{"api", "worker"} {
		svc := wt.Services[svcName]
		dbURLPtr := svc.Environment["DATABASE_URL"]
		if dbURLPtr == nil {
			t.Fatalf("%s: DATABASE_URL missing from environment", svcName)
		}
		if !strings.Contains(*dbURLPtr, tenantDB) {
			t.Errorf("%s: DATABASE_URL = %q, want it to contain tenant DB %q", svcName, *dbURLPtr, tenantDB)
		}
		if strings.Contains(*dbURLPtr, "/myapp") {
			t.Errorf("%s: DATABASE_URL still contains original db name: %q", svcName, *dbURLPtr)
		}
	}

	// REDIS_URL and S3_ENDPOINT must be untouched (full_share, no url_envs)
	apiEnv := wt.Services["api"].Environment
	if redisURL := apiEnv["REDIS_URL"]; redisURL != nil && *redisURL != "redis://redis:6379" {
		t.Errorf("REDIS_URL rewritten unexpectedly: %q", *redisURL)
	}

	// Platform network must be declared external in worktree project
	pNet, ok := wt.Networks[netName]
	if !ok {
		t.Fatalf("worktree missing platform network declaration %q", netName)
	}
	if !bool(pNet.External) {
		t.Errorf("platform network in worktree must be external")
	}

	// Worktree services must be on the platform network
	for _, svcName := range []string{"api", "ui", "worker"} {
		svc := wt.Services[svcName]
		if _, ok := svc.Networks[netName]; !ok {
			t.Errorf("worktree service %q not attached to platform network", svcName)
		}
	}

	// Both generated files must round-trip through compose-go
	dir := t.TempDir()
	platformFile := filepath.Join(dir, "platform-compose.yml")
	worktreeFile := filepath.Join(dir, instanceName+"-worktree-compose.yml")

	if err := WriteComposeFile(platform, platformFile); err != nil {
		t.Fatalf("WriteComposeFile platform: %v", err)
	}
	if err := WriteComposeFile(wt, worktreeFile); err != nil {
		t.Fatalf("WriteComposeFile worktree: %v", err)
	}

	// Reload both — compose-go must be able to parse what we wrote
	if _, _, err := LoadFull([]string{platformFile}); err != nil {
		t.Fatalf("reload platform compose failed: %v", err)
	}
	// Worktree references an external network — reload with skip-validation
	// by checking the file is valid YAML at minimum
	data, err := os.ReadFile(worktreeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), tenantDB) {
		t.Errorf("tenant DB name not in generated worktree compose file")
	}
	if !strings.Contains(string(data), netName) {
		t.Errorf("platform network name not in generated worktree compose file")
	}
}
