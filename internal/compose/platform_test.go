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

func TestEscapeDollar(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"", ""},
		{"no dollars", "no dollars"},
		{"$VAR", "$$VAR"},
		{"hello $WORLD", "hello $$WORLD"},
		{"$$already", "$$already"},
		{"$FOO $BAR", "$$FOO $$BAR"},
		{"mixed $X and $$Y", "mixed $$X and $$Y"},
		{"$X$$Y", "$$X$$Y"},
		{"$$", "$$"},
		{"$$$", "$$$$"},
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
}
