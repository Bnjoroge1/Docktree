package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnjoroge/docktree/internal/config"
)

// helper: write a compose file to temp dir, load full project, return raw + cleanup
func loadRaw(t *testing.T, yml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSynthesizePlatformBasic(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: my-api:latest
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      DATABASE_URL: postgres://db:5432/myapp
    depends_on:
      - db
  db:
    image: postgres:15-alpine
    ports:
      - "127.0.0.1:5432:5432"
    environment:
      POSTGRES_PASSWORD: secret
    volumes:
      - db_data:/var/lib/postgresql/data
volumes:
  db_data:
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}

	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database"},
	}}
	platform, err := SynthesizePlatform(raw, shared, "myrepo")
	if err != nil {
		t.Fatalf("SynthesizePlatform: %v", err)
	}

	if platform.Name != "docktree-platform-myrepo" {
		t.Fatalf("project name = %q", platform.Name)
	}
	if _, ok := platform.Services["db"]; !ok {
		t.Fatalf("db missing from platform services: %v", SortedServiceNames(platform))
	}
	if _, ok := platform.Services["api"]; ok {
		t.Fatalf("api should not be in platform stack")
	}
	db := platform.Services["db"]
	if len(db.Ports) != 0 {
		t.Fatalf("ports should be stripped on platform side: %#v", db.Ports)
	}
	netName := PlatformNetworkName("myrepo")
	netCfg, ok := db.Networks[netName]
	if !ok || netCfg == nil {
		t.Fatalf("db not attached to platform network %q: %#v", netName, db.Networks)
	}
	if len(netCfg.Aliases) != 1 || netCfg.Aliases[0] != "db" {
		t.Fatalf("expected alias [db], got %v", netCfg.Aliases)
	}
	if db.Labels["docktree.tier"] != "platform" || db.Labels["docktree.shared.kind"] != "postgres" {
		t.Fatalf("labels: %#v", db.Labels)
	}
	if db.ContainerName != "docktree-platform-myrepo-db" {
		t.Fatalf("container name: %q", db.ContainerName)
	}

	// Platform network must be declared external
	pNet, ok := platform.Networks[netName]
	if !ok {
		t.Fatalf("platform network missing")
	}
	if !bool(pNet.External) {
		t.Fatalf("platform network must be external: %#v", pNet)
	}

	// db_data should be carried over because the kept service references it
	if _, ok := platform.Volumes["db_data"]; !ok {
		t.Fatalf("db_data volume should be carried into platform project")
	}
}

func TestSynthesizePlatformAliases(t *testing.T) {
	path := loadRaw(t, `
services:
  db:
    image: postgres:15
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database", Aliases: []string{"database", "primary"}},
	}}
	platform, err := SynthesizePlatform(raw, shared, "r")
	if err != nil {
		t.Fatal(err)
	}
	got := platform.Services["db"].Networks[PlatformNetworkName("r")].Aliases
	want := map[string]bool{"db": true, "database": true, "primary": true}
	if len(got) != 3 {
		t.Fatalf("aliases len = %d, want 3 (%v)", len(got), got)
	}
	for _, a := range got {
		if !want[a] {
			t.Fatalf("unexpected alias %q in %v", a, got)
		}
	}
}

func TestSynthesizeWorktreeStripsPlatformServices(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: api:1
    depends_on:
      - db
      - cache
    environment:
      DATABASE_URL: postgres://db:5432/myapp
  worker:
    image: worker:1
    depends_on:
      - db
  cache:
    image: redis:7
  db:
    image: postgres:15
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db":    {Kind: "postgres", Tenancy: "per_database"},
		"cache": {Kind: "redis", Tenancy: "full_share"},
	}}
	wt, err := SynthesizeWorktree(raw, shared, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := wt.Services["db"]; ok {
		t.Fatalf("db must be removed from worktree project")
	}
	if _, ok := wt.Services["cache"]; ok {
		t.Fatalf("cache must be removed from worktree project")
	}
	api, ok := wt.Services["api"]
	if !ok {
		t.Fatal("api missing")
	}
	if len(api.DependsOn) != 0 {
		t.Fatalf("api depends_on should be empty after pruning (was %v)", api.DependsOn)
	}
	worker := wt.Services["worker"]
	if len(worker.DependsOn) != 0 {
		t.Fatalf("worker depends_on should be empty after pruning (was %v)", worker.DependsOn)
	}
	// remaining services should be on the platform network
	netName := PlatformNetworkName("myrepo")
	for name, svc := range wt.Services {
		if _, ok := svc.Networks[netName]; !ok {
			t.Fatalf("%s not attached to platform network %q", name, netName)
		}
	}
	platformNet, ok := wt.Networks[netName]
	if !ok {
		t.Fatal("worktree compose missing platform network declaration")
	}
	if !bool(platformNet.External) {
		t.Fatalf("platform network must be external: %#v", platformNet)
	}
	if _, ok := wt.Networks["default"]; !ok {
		t.Fatal("default network should exist for worktree project")
	}
}

func TestSynthesizeWorktreeWithoutSharedIsIdentity(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: api:1
    depends_on:
      - db
  db:
    image: postgres:15
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	wt, err := SynthesizeWorktree(raw, config.SharedConfig{}, "r")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := wt.Services["api"]; !ok {
		t.Fatal("api missing")
	}
	if _, ok := wt.Services["db"]; !ok {
		t.Fatal("db should remain when no shared services declared")
	}
	api := wt.Services["api"]
	if _, ok := api.DependsOn["db"]; !ok {
		t.Fatalf("depends_on should be preserved untouched (got %v)", api.DependsOn)
	}
}

func TestWriteComposeFileRoundTrip(t *testing.T) {
	path := loadRaw(t, `
services:
  db:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: secret
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database"},
	}}
	platform, err := SynthesizePlatform(raw, shared, "demo")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "platform-compose.yml")
	if err := WriteComposeFile(platform, out); err != nil {
		t.Fatalf("WriteComposeFile: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"docktree-platform-demo-net",
		"postgres:15",
		"docktree.tier:",
		"platform",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated compose missing %q:\n%s", want, text)
		}
	}
	// Sanity: compose-go must be able to reload what we just wrote.
	if _, _, err := LoadFull([]string{out}); err != nil {
		t.Fatalf("re-loading generated platform compose failed: %v\n%s", err, text)
	}
}

func TestGeneratedWorktreeComposePreservesRawSecretsAndEnvFiles(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-live-secret")
	path := loadRaw(t, `
services:
  api:
    image: app
    env_file:
      - .env
    command: bash -c 'echo ${OPENAI_API_KEY} $$POSTGRES_DB'
    entrypoint: sh -c 'echo ${OPENAI_API_KEY}'
    environment:
      OPENAI_API_KEY: ${OPENAI_API_KEY}
      POSTGRES_DB: app
    depends_on:
      - db
  db:
    image: postgres
`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env"), []byte("OPENAI_API_KEY=sk-live-secret\nENV_FILE_ONLY_SECRET=env-file-only-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clean, err := LoadFullClean([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database", DBNameEnvs: []string{"POSTGRES_DB"}},
	}}
	wt, err := SynthesizeWorktree(clean, shared, "demo", SynthesizeWorktreeOptions{
		TenantDBs: map[string]map[string]string{"db": {"": "demo_feature_app"}},
		RawInput:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(filepath.Dir(path), ".docktree", "generated", "demo-worktree-compose.yml")
	if err := RebaseEnvFiles(wt, out); err != nil {
		t.Fatal(err)
	}
	if err := WriteComposeFile(wt, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, secret := range []string{"sk-live-secret", "env-file-only-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("generated compose leaked resolved secret %q:\n%s", secret, text)
		}
	}
	if strings.Contains(text, "ENV_FILE_ONLY_SECRET") {
		t.Fatalf("generated compose leaked resolved secret:\n%s", text)
	}
	for _, want := range []string{"${OPENAI_API_KEY}", "$$POSTGRES_DB", "../../.env", "POSTGRES_DB: demo_feature_app"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated compose missing %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"$${OPENAI_API_KEY}", "$$$$POSTGRES_DB"} {
		if strings.Contains(text, bad) {
			t.Fatalf("generated compose over-escaped %q:\n%s", bad, text)
		}
	}
}

func TestGeneratedPlatformComposePreservesRawSecretsAndEnvFiles(t *testing.T) {
	t.Setenv("INFISICAL_TOKEN", "live-infisical-token")
	path := loadRaw(t, `
services:
  db:
    image: postgres
    env_file:
      - .env
    command: infisical run -- bash -c 'echo ${INFISICAL_TOKEN}'
    environment:
      INFISICAL_TOKEN: ${INFISICAL_TOKEN}
`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env"), []byte("INFISICAL_TOKEN=live-infisical-token\nENV_FILE_ONLY_SECRET=platform-env-file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clean, err := LoadFullClean([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database"},
	}}
	platform, err := SynthesizePlatform(clean, shared, "demo")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(filepath.Dir(path), ".docktree", "generated", "platform-compose.yml")
	if err := RebaseEnvFiles(platform, out); err != nil {
		t.Fatal(err)
	}
	if err := WriteComposeFile(platform, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, secret := range []string{"live-infisical-token", "platform-env-file-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("generated platform compose leaked resolved secret %q:\n%s", secret, text)
		}
	}
	if strings.Contains(text, "ENV_FILE_ONLY_SECRET") {
		t.Fatalf("generated platform compose materialized env_file-only key:\n%s", text)
	}
	for _, want := range []string{"${INFISICAL_TOKEN}", "../../.env"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated platform compose missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "$${INFISICAL_TOKEN}") {
		t.Fatalf("generated platform compose over-escaped placeholder:\n%s", text)
	}
}

func TestGeneratedFilteredComposePreservesRawSecretsAndEnvFiles(t *testing.T) {
	t.Setenv("AUTH_PASSWORD", "resolved-auth-secret")
	path := loadRaw(t, `
services:
  api:
    image: app
    env_file:
      - .env
    command: bash -c 'echo ${AUTH_PASSWORD}'
    environment:
      AUTH_PASSWORD: ${AUTH_PASSWORD}
    depends_on:
      - worker
  worker:
    image: worker
`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env"), []byte("AUTH_PASSWORD=resolved-auth-secret\nENV_FILE_ONLY_SECRET=filtered-env-file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clean, err := LoadFullClean([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := FilterServices(clean, ServiceFilter{Skip: []string{"worker"}})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(filepath.Dir(path), ".docktree", "generated", "demo-filtered.yml")
	if err := RebaseEnvFiles(filtered, out); err != nil {
		t.Fatal(err)
	}
	if err := WriteComposeFile(filtered, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, secret := range []string{"resolved-auth-secret", "filtered-env-file-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("generated filtered compose leaked resolved secret %q:\n%s", secret, text)
		}
	}
	if strings.Contains(text, "ENV_FILE_ONLY_SECRET") {
		t.Fatalf("generated filtered compose materialized env_file-only key:\n%s", text)
	}
	for _, want := range []string{"${AUTH_PASSWORD}", "../../.env"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated filtered compose missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "$${AUTH_PASSWORD}") {
		t.Fatalf("generated filtered compose over-escaped placeholder:\n%s", text)
	}
}

func TestRawURLEnvIsolationWarningsOnlyForResolvedURL(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: app
    env_file:
      - .env
  worker:
    image: worker
    env_file:
      - worker.env
`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env"), []byte("DATABASE_URL=postgres://user:pass@db:5432/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "worker.env"), []byte("OTHER=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	clean, err := LoadFullClean([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}},
	}}
	warnings := RawURLEnvIsolationWarnings(resolved, clean, shared)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want exactly one warning for api", warnings)
	}
	if warnings[0].Key != "shared.url_envs.api.DATABASE_URL" {
		t.Fatalf("warning key = %q, want api DATABASE_URL", warnings[0].Key)
	}
}

func TestRawInputSkipsURLRewriteWhenURLContainsPlaceholders(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: app
    environment:
      DATABASE_URL: postgres://${DB_USER}:${DB_PASS}@db:5432/app
    depends_on:
      - db
  db:
    image: postgres
`)
	clean, err := LoadFullClean([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database", URLEnvs: []string{"DATABASE_URL"}},
	}}
	wt, err := SynthesizeWorktree(clean, shared, "demo", SynthesizeWorktreeOptions{
		TenantDBs: map[string]map[string]string{"db": {"": "demo_feature_app"}},
		RawInput:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(filepath.Dir(path), ".docktree", "generated", "demo-worktree-compose.yml")
	if err := WriteComposeFile(wt, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	want := `DATABASE_URL: postgres://${DB_USER}:${DB_PASS}@db:5432/app`
	if !strings.Contains(text, want) {
		t.Fatalf("generated compose missing unchanged placeholder URL %q:\n%s", want, text)
	}
	for _, bad := range []string{"demo_feature_app", "%24%7BDB_USER%7D", "%24%7BDB_PASS%7D"} {
		if strings.Contains(text, bad) {
			t.Fatalf("generated compose unexpectedly contains %q:\n%s", bad, text)
		}
	}
}

func TestEscapeDollar(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"", ""},
		{"no dollars", "no dollars"},
		{"$VAR", "$$VAR"},
		{"hello $WORLD", "hello $$WORLD"},
		{"$$literal", "$$$$literal"},
		{"$FOO $BAR", "$$FOO $$BAR"},
		{"mixed $X and $$Y", "mixed $$X and $$$$Y"},
		{"$X$$Y", "$$X$$$$Y"},
		{"$$", "$$$$"},
		{"$$$", "$$$$$$"},
		{"postgres://$USER:$PASS@host/db", "postgres://$$USER:$$PASS@host/db"},
		{"MYSQL_ROOT_PASSWORD", "MYSQL_ROOT_PASSWORD"},
	}
	for _, tt := range tests {
		got := escapeDollar(tt.input)
		if got != tt.want {
			t.Errorf("escapeDollar(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSynthesizeWorktreeEscapesDollars(t *testing.T) {
	password := "shh"
	cmd1 := "echo"
	path := loadRaw(t, `
services:
  api:
    image: api:1
    environment:
      SECRET: shh
      DATABASE_URL: postgres://$$DB_USER:$$DB_PASS@db:5432/mydb
    command:
      - echo
      - sh -c 'echo $$HOME'
    entrypoint:
      - /bin/sh
      - -c
      - exec $$MY_APP $$PORT
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"redis": {Kind: "redis", Tenancy: "full_share"},
	}}
	wt, err := SynthesizeWorktree(raw, shared, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	api, ok := wt.Services["api"]
	if !ok {
		t.Fatal("api missing")
	}
	if api.Environment == nil {
		t.Fatal("environment missing")
	}
	if api.Environment["SECRET"] == nil {
		t.Fatal("SECRET env missing")
	}
	if *api.Environment["SECRET"] != password {
		t.Errorf("SECRET = %q, want %q", *api.Environment["SECRET"], password)
	}
	if api.Environment["DATABASE_URL"] == nil {
		t.Fatal("DATABASE_URL env missing")
	}
	gotURL := *api.Environment["DATABASE_URL"]
	if gotURL != escapeDollar("postgres://$DB_USER:$DB_PASS@db:5432/mydb") {
		t.Errorf("DATABASE_URL = %q, want %q", gotURL, escapeDollar("postgres://$DB_USER:$DB_PASS@db:5432/mydb"))
	}
	gotCmd := api.Command
	if len(gotCmd) != 2 {
		t.Fatalf("command should have 2 parts, got %v", gotCmd)
	}
	if gotCmd[0] != cmd1 {
		t.Errorf("command[0] = %q, want %q", gotCmd[0], cmd1)
	}
	wantCmd2 := escapeDollar("sh -c 'echo $HOME'")
	if gotCmd[1] != wantCmd2 {
		t.Errorf("command[1] = %q, want %q", gotCmd[1], wantCmd2)
	}
	gotEP := api.Entrypoint
	if len(gotEP) != 3 {
		t.Fatalf("entrypoint should have 3 parts, got %v", gotEP)
	}
	wantEP2 := escapeDollar("-c")
	wantEP3 := escapeDollar("exec $MY_APP $PORT")
	if gotEP[1] != wantEP2 {
		t.Errorf("entrypoint[1] = %q, want %q", gotEP[1], wantEP2)
	}
	if gotEP[2] != wantEP3 {
		t.Errorf("entrypoint[2] = %q, want %q", gotEP[2], wantEP3)
	}
}

func TestSynthesizeWorktreePreservesPorts(t *testing.T) {
	path := loadRaw(t, `
services:
  api:
    image: api:1
    ports:
      - "127.0.0.1:8080:3000"
      - "9090:9090"
  db:
    image: postgres:15
    ports:
      - "5432:5432"
`)
	raw, _, err := LoadFull([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	shared := config.SharedConfig{Services: map[string]config.SharedService{
		"db": {Kind: "postgres", Tenancy: "per_database"},
	}}
	wt, err := SynthesizeWorktree(raw, shared, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	api, ok := wt.Services["api"]
	if !ok {
		t.Fatal("api missing")
	}
	if len(api.Ports) == 0 {
		t.Fatal("api ports should be preserved for dynamic allocation")
	}
	if _, ok := wt.Services["db"]; ok {
		t.Fatal("db should be removed (it's a platform service)")
	}
}
