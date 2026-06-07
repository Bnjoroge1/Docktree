package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSharedAccepts(t *testing.T) {
	cases := map[string]SharedService{
		"db":     {Kind: "postgres", Tenancy: "per_database", Template: "myapp_base"},
		"db2":    {Kind: "postgres", Tenancy: "full_share"},
		"mysql":  {Kind: "mysql", Tenancy: "per_database", Databases: map[string]SharedDatabase{"app": {URLEnvs: []string{"DATABASE_URL"}}}},
		"redis":  {Kind: "redis", Tenancy: "full_share"},
		"bucket": {Kind: "s3", Tenancy: "full_share"},
		"misc":   {Kind: "generic", Tenancy: "full_share"},
	}
	if err := ValidateShared(SharedConfig{Services: cases}, nil); err != nil {
		t.Fatalf("expected valid config to pass, got %v", err)
	}
}

func TestValidateSharedRejects(t *testing.T) {
	tests := []struct {
		name      string
		shared    SharedConfig
		volShare  []string
		wantSub   string
	}{
		{
			name:    "missing kind",
			shared:  SharedConfig{Services: map[string]SharedService{"db": {Tenancy: "per_database"}}},
			wantSub: "kind is required",
		},
		{
			name:    "unknown kind",
			shared:  SharedConfig{Services: map[string]SharedService{"db": {Kind: "neo4j", Tenancy: "full_share"}}},
			wantSub: "not supported",
		},
		{
			name:    "missing tenancy",
			shared:  SharedConfig{Services: map[string]SharedService{"db": {Kind: "postgres"}}},
			wantSub: "tenancy is required",
		},
		{
			name:    "tenancy not valid for kind",
			shared:  SharedConfig{Services: map[string]SharedService{"r": {Kind: "redis", Tenancy: "per_database"}}},
			wantSub: "not valid for kind",
		},
		{
			name:    "template on non-sql kind",
			shared:  SharedConfig{Services: map[string]SharedService{"r": {Kind: "redis", Tenancy: "full_share", Template: "x"}}},
			wantSub: "template only applies",
		},
		{
			name: "alias collision between services",
			shared: SharedConfig{Services: map[string]SharedService{
				"db":      {Kind: "postgres", Tenancy: "per_database"},
				"db-rep":  {Kind: "postgres", Tenancy: "full_share", Aliases: []string{"db"}},
			}},
			wantSub: "alias",
		},
		{
			name: "mixed legacy and databases",
			shared: SharedConfig{Services: map[string]SharedService{"db": {
				Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}, Databases: map[string]SharedDatabase{"app": {URLEnvs: []string{"DB_CONNECTION_URI"}}},
			}}},
			wantSub: "cannot mix",
		},
		{
			name: "databases require per_database",
			shared: SharedConfig{Services: map[string]SharedService{"db": {
				Kind: "postgres", Tenancy: "full_share", Databases: map[string]SharedDatabase{"app": {URLEnvs: []string{"DATABASE_URL"}}},
			}}},
			wantSub: "requires tenancy per_database",
		},
		{
			name: "database entry requires url envs",
			shared: SharedConfig{Services: map[string]SharedService{"db": {
				Kind: "postgres", Tenancy: "per_database", Databases: map[string]SharedDatabase{"app": {}},
			}}},
			wantSub: "must declare at least one env var",
		},
		{
			name: "empty url_env entry in databases",
			shared: SharedConfig{Services: map[string]SharedService{"db": {
				Kind: "postgres", Tenancy: "per_database", Databases: map[string]SharedDatabase{"app": {URLEnvs: []string{""}}},
			}}},
			wantSub: "cannot contain empty entries",
		},
		{
			name: "duplicate url env across services",
			shared: SharedConfig{Services: map[string]SharedService{
				"a": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}},
				"b": {Kind: "postgres", Tenancy: "per_database", Databases: map[string]SharedDatabase{"app": {URLEnvs: []string{"DATABASE_URL"}}}},
			}},
			wantSub: "claimed by both",
		},
		{
			name: "duplicate url env across logical dbs",
			shared: SharedConfig{Services: map[string]SharedService{"db": {
				Kind: "postgres", Tenancy: "per_database", Databases: map[string]SharedDatabase{
					"app": {URLEnvs: []string{"DATABASE_URL"}},
					"infisical": {URLEnvs: []string{"DATABASE_URL"}},
				},
			}}},
			wantSub: "claimed by both",
		},

		{
			name:     "volume.share overlap",
			shared:   SharedConfig{Services: map[string]SharedService{"db": {Kind: "postgres", Tenancy: "per_database"}}},
			volShare: []string{"db"},
			wantSub:  "both shared.services and volumes.share",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateShared(tc.shared, tc.volShare)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoadParsesSharedMap(t *testing.T) {
	dir := t.TempDir()
	yml := `shared:
  services:
    db:
      kind: postgres
      tenancy: per_database
      tenant_env: DOCKTREE_DB
      aliases: [database]
      databases:
        app:
          url_envs: [DATABASE_URL]
          template: myapp_base
        infisical:
          url_envs: [DB_CONNECTION_URI]
    redis:
      kind: redis
      tenancy: full_share
`
	if err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	db, ok := cfg.Shared.Services["db"]
	if !ok {
		t.Fatalf("db service not parsed: %#v", cfg.Shared.Services)
	}
	if db.Kind != "postgres" || db.Tenancy != "per_database" || db.TenantEnv != "DOCKTREE_DB" || db.Template != "" {
		t.Fatalf("db parsed wrong: %#v", db)
	}
	if len(db.Databases) != 2 || db.Databases["app"].Template != "myapp_base" {
		t.Fatalf("db databases parsed wrong: %#v", db.Databases)
	}
	if len(db.Aliases) != 1 || db.Aliases[0] != "database" {
		t.Fatalf("aliases parsed wrong: %#v", db.Aliases)
	}
	if _, ok := cfg.Shared.Services["redis"]; !ok {
		t.Fatalf("redis service missing")
	}
}

func TestLoadRejectsInvalidShared(t *testing.T) {
	dir := t.TempDir()
	yml := `shared:
  services:
    bad:
      kind: postgres
      tenancy: wrong_mode
`
	if err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected Load to reject invalid tenancy")
	}
}

func TestDefaultTenantEnv(t *testing.T) {
	cases := map[string]string{
		"postgres": "DOCKTREE_DB",
		"mysql":    "DOCKTREE_DB",
		"redis":    "REDIS_KEY_PREFIX",
		"s3":       "S3_BUCKET",
		"unknown":  "",
	}
	for kind, want := range cases {
		if got := DefaultTenantEnv(kind); got != want {
			t.Fatalf("DefaultTenantEnv(%s) = %q, want %q", kind, got, want)
		}
	}
}
