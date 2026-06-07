package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectCapturesServicesAndPorts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yml")
	data := []byte(`
services:
  web:
    image: nginx:alpine
    container_name: myapp_web
    ports:
      - "127.0.0.1:8080:80/tcp"
      - target: 443
        published: "8443"
        host_ip: 127.0.0.1
        protocol: tcp
    environment:
      API_KEY: test
    networks:
      default: {}
  api:
    build: ./api
    image: myapp/api:latest
    depends_on:
      - web
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := LoadProject([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	web := project.Services["web"]
	if web.ContainerName != "myapp_web" || web.Ports[0].Published != 8080 || web.Ports[0].Target != 80 {
		t.Fatalf("web not loaded correctly: %#v", web)
	}
	wantContext := filepath.Join(dir, "api")
	if project.Services["api"].Build == nil || project.Services["api"].Build.Context != wantContext {
		t.Fatalf("build not loaded: %#v, want context %q", project.Services["api"].Build, wantContext)
	}
}

func TestLoadProjectInterpolatesEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yml")
	t.Setenv("WEB_PORT", "18080")
	data := []byte(`
services:
  web:
    image: nginx:alpine
    ports:
      - "127.0.0.1:${WEB_PORT}:80"
    environment:
      API_BASE: ${API_BASE_URL:-http://localhost:3000}
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := LoadProject([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	web := project.Services["web"]
	if got := web.Ports[0].Published; got != 18080 {
		t.Fatalf("published port = %d, want 18080", got)
	}
	if got := web.Environment["API_BASE"]; got != "http://localhost:3000" {
		t.Fatalf("API_BASE = %q, want default interpolation", got)
	}
}

func TestLoadProjectSupportsContainerOnlyPortSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yml")
	data := []byte(`
services:
  redis:
    image: redis:7
    ports:
      - "6379"
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := LoadProject([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	redis := project.Services["redis"]
	if len(redis.Ports) != 1 {
		t.Fatalf("ports = %#v, want one port", redis.Ports)
	}
	if got := redis.Ports[0].Target; got != 6379 {
		t.Fatalf("target = %d, want 6379", got)
	}
	if got := redis.Ports[0].Published; got != 0 {
		t.Fatalf("published = %d, want 0 for container-only syntax", got)
	}
}

func TestLoadProjectMergesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "compose.yml")
	override := filepath.Join(dir, "compose.override.yml")
	if err := os.WriteFile(base, []byte(`
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(override, []byte(`
services:
  web:
    environment:
      MODE: override
`), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := LoadProject([]string{base, override})
	if err != nil {
		t.Fatal(err)
	}
	web := project.Services["web"]
	if web.Image != "nginx:alpine" {
		t.Fatalf("image = %q, want nginx:alpine", web.Image)
	}
	if web.Environment["MODE"] != "override" {
		t.Fatalf("environment = %#v, want MODE from override", web.Environment)
	}
	if len(web.Ports) != 1 || web.Ports[0].Published != 8080 {
		t.Fatalf("ports = %#v, want base port preserved", web.Ports)
	}
}

func TestLoadProjectDedupesIdenticalPortsAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "compose.yml")
	extra := filepath.Join(dir, "extra.yml")
	// Same service publishes the same host:container port in both files,
	// one via short syntax and one via long syntax. Compose merge concatenates
	// the port arrays; Docktree must collapse the exact duplicate so the
	// generated override is not rejected for non-unique ports.
	if err := os.WriteFile(base, []byte(`
services:
  db:
    image: postgres:15-alpine
    ports:
      - "127.0.0.1:5432:5432"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte(`
services:
  db:
    ports:
      - target: 5432
        published: 5432
        host_ip: 127.0.0.1
`), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := LoadProject([]string{base, extra})
	if err != nil {
		t.Fatal(err)
	}
	db := project.Services["db"]
	if len(db.Ports) != 1 {
		t.Fatalf("ports = %#v, want 1 deduped entry", db.Ports)
	}
	if db.Ports[0].Target != 5432 || db.Ports[0].Published != 5432 || db.Ports[0].HostIP != "127.0.0.1" {
		t.Fatalf("port = %#v, want 127.0.0.1:5432:5432", db.Ports[0])
	}
}


func TestLoadFullMaterializesEnvFileIntoServiceEnvironment(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "infisical.env")
	if err := os.WriteFile(envPath, []byte("DB_CONNECTION_URI=postgres://infisical:secret@db:5432/infisical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(composePath, []byte(`
services:
  infisical:
    image: infisical/infisical:latest
    env_file:
      - infisical.env
`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, reduced, err := LoadFull([]string{composePath})
	if err != nil {
		t.Fatal(err)
	}
	rawSvc, ok := raw.Services["infisical"]
	if !ok {
		t.Fatalf("raw service infisical missing: %#v", raw.Services)
	}
	rawURL := rawSvc.Environment["DB_CONNECTION_URI"]
	if rawURL == nil || *rawURL != "postgres://infisical:secret@db:5432/infisical" {
		t.Fatalf("raw DB_CONNECTION_URI = %v, want materialized env_file value", rawURL)
	}
	if got := reduced.Services["infisical"].Environment["DB_CONNECTION_URI"]; got != "postgres://infisical:secret@db:5432/infisical" {
		t.Fatalf("reduced DB_CONNECTION_URI = %q, want env_file value", got)
	}
}
