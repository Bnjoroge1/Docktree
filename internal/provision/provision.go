// Package provision handles per-worktree tenant setup inside shared platform
// services. Each provisioner is idempotent: calling it when the tenant
// already exists is a no-op.
package provision

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TenantConfig carries everything a provisioner needs to create or verify
// one worktree's logical namespace inside a shared service.
type TenantConfig struct {
	// Kind matches config.SharedService.Kind (postgres, mysql, mongodb, redis, s3, generic).
	Kind string
	// Tenancy matches config.SharedService.Tenancy (per_database, full_share)
	Tenancy string
	// TenantName is the computed per-worktree identifier: the database name
	// for postgres/mysql/mongodb, the bucket name for s3, etc.
	TenantName string
	// Template is the optional source database for CREATE DATABASE ... TEMPLATE.
	// Postgres only; empty means no template.
	Template string
	// Host is the hostname of the platform service container.
	Host string
	// Port is the port the service listens on inside the platform network.
	Port     int
	User     string
	Password string
}

type tenantDriver interface {
	Provision(TenantConfig) error
	Deprovision(TenantConfig) error
	Exists(TenantConfig) (bool, error)
	Wait(TenantConfig, int) error
}

var tenantDrivers = map[string]tenantDriver{
	"postgres": postgresDriver{},
	"mysql":    mysqlDriver{},
	"mongodb":  mongoDriver{},
}

// Provision creates the per-worktree tenant inside the shared service, or verifies it already exists. It is safe to call multiple times.
// It shells out to the appropriate CLI tool rather than pulling in a database SDK, keeping the binary dependency surface small.
func Provision(cfg TenantConfig) error {
	if cfg.Tenancy != "per_database" {
		return nil
	}
	driver, ok := tenantDrivers[cfg.Kind]
	if !ok {
		if isNoopKind(cfg.Kind) {
			return nil
		}
		return fmt.Errorf("provision: unknown kind %q", cfg.Kind)
	}
	return driver.Provision(cfg)
}

// Deprovision drops the per-worktree tenant namespace. Called only on
// explicit destructive paths (down -v, platform clean --tenant).
func Deprovision(cfg TenantConfig) error {
	if cfg.Tenancy != "per_database" {
		return nil
	}
	driver, ok := tenantDrivers[cfg.Kind]
	if !ok {
		if isNoopKind(cfg.Kind) {
			return nil
		}
		return fmt.Errorf("deprovision: unknown kind %q", cfg.Kind)
	}
	return driver.Deprovision(cfg)
}

func DBExists(cfg TenantConfig) (bool, error) {
	driver, ok := tenantDrivers[cfg.Kind]
	if !ok {
		return false, fmt.Errorf("exists: unknown kind %q", cfg.Kind)
	}
	return driver.Exists(cfg)
}

func WaitForService(cfg TenantConfig, timeoutSec int) error {
	driver, ok := tenantDrivers[cfg.Kind]
	if !ok {
		return nil
	}
	return driver.Wait(cfg, timeoutSec)
}

func isNoopKind(kind string) bool {
	switch kind {
	case "redis", "s3", "generic":
		return true
	default:
		return false
	}
}

// escapePostgresIdentifier escapes a PostgreSQL identifier by doubling
// embedded double-quote characters, safe for use inside "..." quotes.
func escapePostgresIdentifier(value string) string {
	return strings.ReplaceAll(value, `"`, `""`)
}

// escapePostgresLiteral escapes a PostgreSQL string literal by doubling
// embedded single-quote characters, safe for use inside '...' quotes.
func escapePostgresLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

type postgresDriver struct{}

func (postgresDriver) Provision(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("provision postgres: empty tenant name")
	}
	exists, err := postgresDBExists(cfg.Host, cfg.TenantName, cfg.User, cfg.Password)
	if err != nil {
		return fmt.Errorf("provision postgres: checking database: %w", err)
	}
	if exists {
		return nil
	}

	var stmt string
	if cfg.Template != "" {
		stmt = fmt.Sprintf(`CREATE DATABASE "%s" TEMPLATE "%s"`, escapePostgresIdentifier(cfg.TenantName), escapePostgresIdentifier(cfg.Template))
	} else {
		stmt = fmt.Sprintf(`CREATE DATABASE "%s"`, escapePostgresIdentifier(cfg.TenantName))
	}
	return postgresExec(cfg.Host, cfg.User, cfg.Password, stmt)
}

func (postgresDriver) Deprovision(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("deprovision postgres: empty tenant name")
	}
	exists, err := postgresDBExists(cfg.Host, cfg.TenantName, cfg.User, cfg.Password)
	if err != nil {
		return fmt.Errorf("deprovision postgres: checking database: %w", err)
	}
	if !exists {
		return nil
	}
	terminate := fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='%s' AND pid <> pg_backend_pid()`,
		escapePostgresLiteral(cfg.TenantName),
	)
	_ = postgresExec(cfg.Host, cfg.User, cfg.Password, terminate)
	drop := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, escapePostgresIdentifier(cfg.TenantName))
	return postgresExec(cfg.Host, cfg.User, cfg.Password, drop)
}

func (postgresDriver) Exists(cfg TenantConfig) (bool, error) {
	return postgresDBExists(cfg.Host, cfg.TenantName, cfg.User, cfg.Password)
}

func (postgresDriver) Wait(cfg TenantConfig, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	var lastErr error
	for i := 0; i < timeoutSec*2; i++ {
		cmd := exec.Command("docker", "exec", cfg.Host, "pg_isready", "-U", cfg.User, "-q")
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("postgres in %s not ready after %ds: %w", cfg.Host, timeoutSec, lastErr)
}

func postgresDBExists(container, dbName, user, password string) (bool, error) {
	query := fmt.Sprintf(`SELECT 1 FROM pg_database WHERE datname='%s'`, escapePostgresLiteral(dbName))
	out, err := psql(container, "postgres", user, password, query)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "1"), nil
}

func postgresExec(container, user, password, stmt string) error {
	_, err := psql(container, "postgres", user, password, stmt)
	return err
}

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

type mysqlDriver struct{}

func (mysqlDriver) Provision(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("provision mysql: empty tenant name")
	}
	exists, err := mysqlDBExists(cfg.Host, cfg.TenantName, cfg.User, cfg.Password)
	if err != nil {
		return fmt.Errorf("provision mysql: checking database: %w", err)
	}
	if exists {
		return nil
	}
	if cfg.Template != "" {
		return fmt.Errorf("provision mysql: templates are not supported")
	}
	stmt := fmt.Sprintf("CREATE DATABASE `%s`", escapeMySQLIdentifier(cfg.TenantName))
	return mysqlExec(cfg.Host, cfg.User, cfg.Password, stmt)
}

func (mysqlDriver) Deprovision(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("deprovision mysql: empty tenant name")
	}
	exists, err := mysqlDBExists(cfg.Host, cfg.TenantName, cfg.User, cfg.Password)
	if err != nil {
		return fmt.Errorf("deprovision mysql: checking database: %w", err)
	}
	if !exists {
		return nil
	}
	stmt := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", escapeMySQLIdentifier(cfg.TenantName))
	return mysqlExec(cfg.Host, cfg.User, cfg.Password, stmt)
}

func (mysqlDriver) Exists(cfg TenantConfig) (bool, error) {
	return mysqlDBExists(cfg.Host, cfg.TenantName, cfg.User, cfg.Password)
}

func (mysqlDriver) Wait(cfg TenantConfig, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	var lastErr error
	for i := 0; i < timeoutSec*2; i++ {
		_, err := mysql(cfg.Host, cfg.User, cfg.Password, "SELECT 1")
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("mysql in %s not ready after %ds: %w", cfg.Host, timeoutSec, lastErr)
}

func mysqlDBExists(container, dbName, user, password string) (bool, error) {
	query := fmt.Sprintf("SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = '%s'", escapeSQLString(dbName))
	out, err := mysql(container, user, password, query)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, dbName), nil
}

func mysqlExec(container, user, password, stmt string) error {
	_, err := mysql(container, user, password, stmt)
	return err
}

func mysql(container, user, password, query string) (string, error) {
	args := []string{"exec"}
	if password != "" {
		args = append(args, "-e", "MYSQL_PWD="+password)
	}
	args = append(args,
		container,
		"mysql",
		"-u", user,
		"-N", "-B",
		"-e", query,
	)
	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mysql: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func escapeMySQLIdentifier(value string) string {
	return strings.ReplaceAll(value, "`", "``")
}

func escapeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

type mongoDriver struct{}

const mongoMarkerCollection = "__docktree_tenant"

func (mongoDriver) Provision(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("provision mongodb: empty tenant name")
	}
	if cfg.Template != "" {
		return fmt.Errorf("provision mongodb: templates are not supported")
	}
	script := fmt.Sprintf(
		`const tenant = db.getSiblingDB(%s); if (!tenant.getCollectionNames().includes(%s)) tenant.createCollection(%s);`,
		quoteJSString(cfg.TenantName),
		quoteJSString(mongoMarkerCollection),
		quoteJSString(mongoMarkerCollection),
	)
	_, err := mongosh(cfg.Host, cfg.User, cfg.Password, script)
	return err
}

func (mongoDriver) Deprovision(cfg TenantConfig) error {
	if cfg.TenantName == "" {
		return fmt.Errorf("deprovision mongodb: empty tenant name")
	}
	script := fmt.Sprintf(`db.getSiblingDB(%s).dropDatabase();`, quoteJSString(cfg.TenantName))
	_, err := mongosh(cfg.Host, cfg.User, cfg.Password, script)
	return err
}

func (mongoDriver) Exists(cfg TenantConfig) (bool, error) {
	if cfg.TenantName == "" {
		return false, fmt.Errorf("exists mongodb: empty tenant name")
	}
	script := fmt.Sprintf(
		`const exists = db.adminCommand({listDatabases: 1, nameOnly: true}).databases.some(d => d.name === %s); print(exists ? "1" : "0");`,
		quoteJSString(cfg.TenantName),
	)
	out, err := mongosh(cfg.Host, cfg.User, cfg.Password, script)
	if err != nil {
		return false, err
	}
	// Check only the last non-empty line to avoid false positives from
	// mongosh noise (deprecation notices, connection info, etc.).
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" {
			return l == "1", nil
		}
	}
	return false, nil
}

func (mongoDriver) Wait(cfg TenantConfig, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	var lastErr error
	for i := 0; i < timeoutSec*2; i++ {
		_, err := mongosh(cfg.Host, cfg.User, cfg.Password, `db.adminCommand({ping: 1})`)
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("mongodb in %s not ready after %ds: %w", cfg.Host, timeoutSec, lastErr)
}

func mongosh(container, user, password, script string) (string, error) {
	args := []string{"exec", container, "mongosh", "--quiet"}
	if user != "" {
		args = append(args, "-u", user)
	}
	if password != "" {
		args = append(args, "-p", password)
	}
	if user != "" || password != "" {
		args = append(args, "--authenticationDatabase", "admin")
	}
	args = append(args, "--eval", script)
	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mongosh: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func quoteJSString(value string) string {
	return strconv.Quote(value)
}

// TenantName returns the deterministic tenant identifier for the legacy
// single-database/shared-resource case.
func TenantName(repoSlug, instanceName string) string {
	return TenantNameForDatabase(repoSlug, instanceName, "")
}

// TenantNameForDatabase returns the deterministic database/bucket/key-prefix
// name for a given repo, worktree instance, and optional logical database key.
func TenantNameForDatabase(repoSlug, instanceName, logicalDB string) string {
	// Put logicalDB first so that when the joined slug exceeds common identifier
	// caps, the suffix that gets truncated is the repo/instance tail, not the
	// discriminator between two logical DBs of the same worktree.
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
		slug = strings.TrimRight(slug[:63], "_")
	}
	if slug == "" {
		slug = "docktree"
	}
	return slug
}
