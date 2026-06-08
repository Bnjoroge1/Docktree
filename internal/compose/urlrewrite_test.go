package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnjoroge/docktree/internal/config"
)

func TestRewriteURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		tenant string
		want   string
	}{
		{
			name:   "standard postgres",
			input:  "postgres://db:5432/myapp",
			tenant: "myrepo_branch_abc",
			want:   "postgres://db:5432/myrepo_branch_abc",
		},
		{
			name:   "postgresql scheme",
			input:  "postgresql://db:5432/myapp",
			tenant: "myrepo_branch_abc",
			want:   "postgresql://db:5432/myrepo_branch_abc",
		},
		{
			name:   "with credentials",
			input:  "postgres://user:secret@db:5432/myapp",
			tenant: "tenant_db",
			want:   "postgres://user:secret@db:5432/tenant_db",
		},
		{
			name:   "with query string preserved",
			input:  "postgres://db:5432/myapp?sslmode=require&connect_timeout=10",
			tenant: "tenant_db",
			want:   "postgres://db:5432/tenant_db?sslmode=require&connect_timeout=10",
		},
		{
			name:   "prisma with schema query param",
			input:  "postgresql://user:pass@db:5432/myapp?schema=public",
			tenant: "myrepo_feat",
			want:   "postgresql://user:pass@db:5432/myrepo_feat?schema=public",
		},
		{
			name:   "sqlalchemy dialect prefix",
			input:  "postgresql+asyncpg://db:5432/myapp",
			tenant: "tenant_db",
			want:   "postgresql+asyncpg://db:5432/tenant_db",
		},
		{
			name:   "jdbc prefix",
			input:  "jdbc:postgresql://db:5432/myapp",
			tenant: "tenant_db",
			want:   "jdbc:postgresql://db:5432/tenant_db",
		},
		{
			name:   "mongodb with auth source",
			input:  "mongodb://root:secret@mongo:27017/myapp?authSource=admin",
			tenant: "tenant_db",
			want:   "mongodb://root:secret@mongo:27017/tenant_db?authSource=admin",
		},
		{
			name:   "mongodb srv",
			input:  "mongodb+srv://root:secret@cluster.example.com/myapp",
			tenant: "tenant_db",
			want:   "mongodb+srv://root:secret@cluster.example.com/tenant_db",
		},
		{
			name:   "no port",
			input:  "postgres://db/myapp",
			tenant: "tenant_db",
			want:   "postgres://db/tenant_db",
		},
		{
			name:   "url encoded password",
			input:  "postgres://user:p%40ssword@db:5432/myapp",
			tenant: "tenant_db",
			want:   "postgres://user:p%40ssword@db:5432/tenant_db",
		},
		{
			name:   "empty input unchanged",
			input:  "",
			tenant: "tenant_db",
			want:   "",
		},
		{
			name:   "non-url unchanged",
			input:  "not-a-url",
			tenant: "tenant_db",
			want:   "not-a-url",
		},
		{
			name:   "empty tenant unchanged",
			input:  "postgres://db:5432/myapp",
			tenant: "",
			want:   "postgres://db:5432/myapp",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RewriteURL(tc.input, tc.tenant)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("RewriteURL(%q, %q) = %q, want %q", tc.input, tc.tenant, got, tc.want)
			}
		})
	}
}

func TestRewriteURLEnvs(t *testing.T) {
	envs := map[string]string{
		"DATABASE_URL":         "postgres://db:5432/myapp",
		"DATABASE_REPLICA_URL": "postgres://replica:5432/myapp",
		"REDIS_URL":            "redis://redis:6379",
		"APP_NAME":             "myapp",
	}

	got, err := RewriteURLEnvs(envs, []string{"DATABASE_URL", "DATABASE_REPLICA_URL"}, "myrepo_main_abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got["DATABASE_URL"] != "postgres://db:5432/myrepo_main_abc" {
		t.Fatalf("DATABASE_URL not rewritten: %q", got["DATABASE_URL"])
	}
	if got["DATABASE_REPLICA_URL"] != "postgres://replica:5432/myrepo_main_abc" {
		t.Fatalf("DATABASE_REPLICA_URL not rewritten: %q", got["DATABASE_REPLICA_URL"])
	}
	// These must be untouched
	if got["REDIS_URL"] != "redis://redis:6379" {
		t.Fatalf("REDIS_URL should be unchanged: %q", got["REDIS_URL"])
	}
	if got["APP_NAME"] != "myapp" {
		t.Fatalf("APP_NAME should be unchanged: %q", got["APP_NAME"])
	}
	// Original must not be modified
	if envs["DATABASE_URL"] != "postgres://db:5432/myapp" {
		t.Fatal("RewriteURLEnvs must not modify the input map")
	}
}

func TestRewriteURLEnvsEmptyListNoOp(t *testing.T) {
	envs := map[string]string{"DATABASE_URL": "postgres://db:5432/myapp"}
	got, err := RewriteURLEnvs(envs, nil, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if got["DATABASE_URL"] != "postgres://db:5432/myapp" {
		t.Fatalf("empty url_envs should be a no-op, got %q", got["DATABASE_URL"])
	}
}

func TestSynthesizeWorktreeRewritesURLEnvs(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: api:1
    environment:
      DATABASE_URL: postgres://db:5432/myapp
      REDIS_URL: redis://redis:6379
    depends_on:
      - db
  db:
    image: postgres:15
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}

	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {
			Kind:    "postgres",
			Tenancy: "per_database",
			URLEnvs: []string{"DATABASE_URL"},
		},
	}}

	wt, err := SynthesizeWorktree(raw, shared, "myrepo", SynthesizeWorktreeOptions{
		TenantDBs: map[string]map[string]string{"db": {"": "myrepo_main_abc123"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	api := wt.Services["api"]
	dbURL := api.Environment["DATABASE_URL"]
	if dbURL == nil {
		t.Fatal("DATABASE_URL missing from api env")
	}
	if *dbURL != "postgres://db:5432/myrepo_main_abc123" {
		t.Fatalf("DATABASE_URL = %q, want postgres://db:5432/myrepo_main_abc123", *dbURL)
	}
	// REDIS_URL must be untouched
	redisURL := api.Environment["REDIS_URL"]
	if redisURL == nil || *redisURL != "redis://redis:6379" {
		t.Fatalf("REDIS_URL should be unchanged, got %v", redisURL)
	}
}

func TestSynthesizeWorktreeNoRewriteWhenNoTenantDB(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: api:1
    environment:
      DATABASE_URL: postgres://db:5432/myapp
  db:
    image: postgres:15
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}

	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}},
	}}

	// No TenantDBs passed — URL must be untouched
	wt, err := SynthesizeWorktree(raw, shared, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	api := wt.Services["api"]
	dbURL := api.Environment["DATABASE_URL"]
	if dbURL == nil || *dbURL != "postgres://db:5432/myapp" {
		t.Fatalf("DATABASE_URL should be unchanged, got %v", dbURL)
	}
}

func TestSynthesizeWorktreeRewritesEnvFileBackedURLEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "infisical.env")
	if err := os.WriteFile(envPath, []byte("DB_CONNECTION_URI=postgres://infisical:secret@db:5432/infisical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(`
services:
  infisical:
    image: infisical/infisical:latest
    env_file:
      - infisical.env
    depends_on:
      - db
  db:
    image: postgres:15
`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _, err := LoadFull([]string{composePath})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DB_CONNECTION_URI"}},
	}}
	wt, err := SynthesizeWorktree(raw, shared, "myrepo", SynthesizeWorktreeOptions{
		TenantDBs: map[string]map[string]string{"db": {"": "myrepo_feature_infisical"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	infisical := wt.Services["infisical"]
	dbURL := infisical.Environment["DB_CONNECTION_URI"]
	if dbURL == nil {
		t.Fatal("DB_CONNECTION_URI missing from infisical env")
	}
	if *dbURL != "postgres://infisical:secret@db:5432/myrepo_feature_infisical" {
		t.Fatalf("DB_CONNECTION_URI = %q, want postgres://infisical:secret@db:5432/myrepo_feature_infisical", *dbURL)
	}
}

func TestSynthesizeWorktreeRewritesMultipleLogicalDatabases(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: api:1
    environment:
      DATABASE_URL: postgres://db:5432/app
      DB_CONNECTION_URI: postgres://db:5432/infisical
    depends_on:
      - db
  db:
    image: postgres:15
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {
			Kind:    "postgres",
			Tenancy: "per_database",
			Databases: map[string]config.SharedDatabase{
				"app":       {URLEnvs: []string{"DATABASE_URL"}},
				"infisical": {URLEnvs: []string{"DB_CONNECTION_URI"}},
			},
		},
	}}
	wt, err := SynthesizeWorktree(raw, shared, "myrepo", SynthesizeWorktreeOptions{
		TenantDBs: map[string]map[string]string{"db": {
			"app":       "myrepo_feature_app",
			"infisical": "myrepo_feature_infisical",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	api := wt.Services["api"]
	if got := api.Environment["DATABASE_URL"]; got == nil || *got != "postgres://db:5432/myrepo_feature_app" {
		t.Fatalf("DATABASE_URL = %v", got)
	}
	if got := api.Environment["DB_CONNECTION_URI"]; got == nil || *got != "postgres://db:5432/myrepo_feature_infisical" {
		t.Fatalf("DB_CONNECTION_URI = %v", got)
	}
}
