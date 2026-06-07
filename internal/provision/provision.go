// Package provision handles per-worktree tenant setup inside shared platform
// services. Each provisioner is idempotent: calling it when the tenant
// already exists is a no-op.
package provision

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TenantConfig carries everything a provisioner needs to create or verify
// one worktree's logical namespace inside a shared service.
type TenantConfig struct {
	// Kind matches config.SharedService.Kind (postgres, mysql, redis, s3, generic).
	Kind string
	// Tenancy matches config.SharedService.Tenancy (per_database, full_share)
	Tenancy string
	// TenantName is the computed per-worktree identifier: the database name
	// for postgres/mysql, the bucket name for s3, etc.
	TenantName string
	// Template is the optional source database for CREATE DATABASE ... TEMPLATE.
	// Postgres/mysql only; empty means no template.
	Template string
	// Host is the hostname of the platform service container.
	Host string
	// Port is the port the service listens on inside the platform network.
	Port int
	User     string
	Password string
}

// Provision creates the per-worktree tenant inside the shared service, or verifies it already exists. It is safe to call multiple times.
// It shells out to the appropriate CLI tool rather than pulling in a database SDK, keeping the binary dependency surface small.
func Provision(cfg TenantConfig) error {
	switch cfg.Kind {
	case "postgres", "mysql":
		if cfg.Tenancy != "per_database" {
			return nil // full_share — nothing to provision
		}
		return provisionPostgres(cfg)
	case "redis", "s3", "generic":
		return nil // no provisioning for now, will think about how to add later
	default:
		return fmt.Errorf("provision: unknown kind %q", cfg.Kind)
	}
}

// provisionPostgres creates the tenant database in the shared Postgres cluster
// if it does not already exist. Uses `docker exec` to run psql inside the
// platform service container — no psql required on the host.
func provisionPostgres(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("provision postgres: empty tenant name")
	}
	containerName := "docktree-platform-" + cfg.Host + "-" + "db" // best-effort; callers may override
	// Prefer using the container name set by SynthesizePlatform:
	// docktree-platform-<repo>-<service>. If you that pass the correct container name in cfg.Host can skip this heuristic.
	container := cfg.Host

	// Check if the database already exists.
	exists, err := postgresDBExists(container, cfg.TenantName, cfg.User, cfg.Password)
	if err != nil {
		return fmt.Errorf("provision postgres: checking database: %w", err)
	}
	if exists {
		return nil
	}

	// Build CREATE DATABASE statement.
	var stmt string
	if cfg.Template != "" {
		stmt = fmt.Sprintf(`CREATE DATABASE "%s" TEMPLATE "%s"`, cfg.TenantName, cfg.Template)
	} else {
		stmt = fmt.Sprintf(`CREATE DATABASE "%s"`, cfg.TenantName)
	}
	_ = containerName
	return postgresExec(container, cfg.User, cfg.Password, stmt)
}

// postgresDBExists returns true if the named database exists in the cluster.
func postgresDBExists(container, dbName, user, password string) (bool, error) {
	query := fmt.Sprintf(`SELECT 1 FROM pg_database WHERE datname='%s'`, dbName)
	out, err := psql(container, "postgres", user, password, query)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "1"), nil
}

// postgresExec runs a SQL statement as a superuser, ignoring row output.
func postgresExec(container, user, password, stmt string) error {
	_, err := psql(container, "postgres", user, password, stmt)
	return err
}

// psql shells out via `docker exec` so the host needs no psql binary.
func psql(container, database, user, password, query string) (string, error) {
	args := []string{"exec"}
	if password != "" {
		args = append(args, "-e", "PGPASSWORD="+password)
	}
	args = append(args,
		container,
		"psql",
		"-U", user,
		"-d", database,
		"-t", "-A", 
		"-c", query,
	)
	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// TenantName returns the deterministic tenant identifier for the legacy
// single-database/shared-resource case.
func TenantName(repoSlug, instanceName string) string {
	return TenantNameForDatabase(repoSlug, instanceName, "")
}

// TenantNameForDatabase returns the deterministic database/bucket/key-prefix
// name for a given repo, worktree instance, and optional logical database key.
func TenantNameForDatabase(repoSlug, instanceName, logicalDB string) string {
	// Put logicalDB first so that when the joined slug exceeds Postgres's
	// 63-byte identifier cap, the suffix that gets truncated is the
	// repo/instance tail, not the discriminator between two logical DBs of
	// the same worktree.
	var parts []string
	if logicalDB != "" {
		parts = append(parts, logicalDB)
	}
	parts = append(parts, repoSlug, instanceName)
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		default:
			return '_'
		}
	}, strings.Join(parts, "_"))
	for strings.Contains(slug, "__") {
		slug = strings.ReplaceAll(slug, "__", "_")
	}
	slug = strings.Trim(slug, "_")
	if len(slug) > 63 {
		slug = slug[:63]
	}
	if slug == "" {
		slug = "docktree"
	}
	return slug
}

// Deprovision drops the per-worktree tenant namespace. Called only on
// explicit destructive paths (down -v, platform clean --tenant).
func Deprovision(cfg TenantConfig) error {
	switch cfg.Kind {
	case "postgres", "mysql":
		if cfg.Tenancy != "per_database" {
			return nil
		}
		return dropPostgresDB(cfg)
	case "redis", "s3", "generic":
		return nil //will ad later
	default:
		return fmt.Errorf("deprovision: unknown kind %q", cfg.Kind)
	}
}

func dropPostgresDB(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("deprovision postgres: empty tenant name")
	}
	container := cfg.Host
	exists, err := postgresDBExists(container, cfg.TenantName, cfg.User, cfg.Password)
	if err != nil {
		return fmt.Errorf("deprovision postgres: checking database: %w", err)
	}
	if !exists {
		return nil // already gone
	}
	// Terminate active connections so DROP DATABASE doesn't block.
	terminate := fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='%s' AND pid <> pg_backend_pid()`,
		cfg.TenantName,
	)
	_ = postgresExec(container, cfg.User, cfg.Password, terminate) // best-effort because u can get all sorts of issues like concurrent reconnets etc
	drop := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, cfg.TenantName)
	return postgresExec(container, cfg.User, cfg.Password, drop)
}

// WaitForPostgres polls pg_isready inside the platform container until it
// responds or the timeout elapses. Returns nil when ready.

func DBExists(container, dbName, user, password string) (bool, error) {
	return postgresDBExists(container, dbName, user, password)
}

func WaitForPostgres(container, user string, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	var lastErr error
	for i := 0; i < timeoutSec*2; i++ {
		cmd := exec.Command("docker", "exec", container,
			"pg_isready", "-U", user, "-q")
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		// 500ms between retries
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("postgres in %s not ready after %ds: %w", container, timeoutSec, lastErr)
}
