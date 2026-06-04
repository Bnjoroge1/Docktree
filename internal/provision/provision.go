// Package provision handles per-worktree tenant setup inside shared platform
// services. Each provisioner is idempotent: calling it when the tenant
// already exists is a no-op.
package provision

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// TenantConfig carries everything a provisioner needs to create or verify
// one worktree's logical namespace inside a shared service.
type TenantConfig struct {
	// Kind matches config.SharedService.Kind (postgres, mysql, redis, s3, generic).
	Kind string
	// Tenancy matches config.SharedService.Tenancy (per_database, full_share, …).
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
	// User and Password are the credentials to authenticate with.
	User     string
	Password string
}

// Provision creates the per-worktree tenant inside the shared service, or
// verifies it already exists. It is safe to call multiple times.
//
// It shells out to the appropriate CLI tool rather than pulling in a database
// SDK, keeping the binary dependency surface small.
func Provision(cfg TenantConfig) error {
	switch cfg.Kind {
	case "postgres", "mysql":
		if cfg.Tenancy != "per_database" {
			return nil // full_share — nothing to provision
		}
		return provisionPostgres(cfg)
	case "redis", "s3", "generic":
		return nil // no provisioning in v1 for these kinds
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
	// docktree-platform-<repo>-<service>. Callers that pass the correct
	// container name in cfg.Host can skip this heuristic.
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
// If password is non-empty it is passed via PGPASSWORD env on the exec call.
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
		"-t", "-A", // tuples-only, unaligned — easier to parse
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

// TenantName returns the deterministic database/bucket/key-prefix name for
// a given (repoSlug, instance, serviceKind) triple. Stable across calls.
func TenantName(repoSlug, instanceName string) string {
	// Flatten to a valid Postgres identifier: only lowercase alnum and _.
	// Postgres identifiers max 63 bytes; we stay well under.
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32 // toLower
		default:
			return '_'
		}
	}, repoSlug+"_"+instanceName)
	// Collapse consecutive underscores.
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
