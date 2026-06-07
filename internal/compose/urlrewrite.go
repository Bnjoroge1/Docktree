package compose

import (
	"fmt"
	"net/url"
	"strings"
)

// RewriteURL replaces the database/path component of a connection URL with
// tenantDB, preserving everything else (scheme, credentials, host, port,
// query string).
//
// Handles standard connection URL shapes for Postgres, MySQL, and MariaDB:
//
//	postgres://host:5432/mydb
//	postgresql://user:pass@host:5432/mydb?sslmode=require
//	postgresql+asyncpg://host/mydb          (SQLAlchemy dialect prefix)
//	jdbc:postgresql://host:5432/mydb        (Java — jdbc: prefix stripped/re-added)
//	mysql://user:pass@host:3306/mydb
//	mysql2://user:pass@host:3306/mydb?charset=utf8mb4
//	mariadb://user:pass@host:3306/mydb
//
// Returns the original value unchanged if it cannot be parsed as a URL or
// has no path component to rewrite.
func RewriteURL(raw, tenantDB string) (string, error) {
	if raw == "" || tenantDB == "" {
		return raw, nil
	}

	// JDBC URLs have a non-standard "jdbc:" prefix that url.Parse chokes on.
	// Strip it, rewrite, re-add.
	jdbcPrefix := ""
	s := raw
	if strings.HasPrefix(s, "jdbc:") {
		jdbcPrefix = "jdbc:"
		s = s[len("jdbc:"):]
	}

	// SQLAlchemy dialect suffixes like "postgresql+asyncpg://" confuse
	// url.Parse's scheme detection. Normalise to a plain scheme for parsing.
	dialectSuffix := ""
	if idx := strings.Index(s, "+"); idx > 0 && idx < strings.Index(s, "://") {
		schemeEnd := strings.Index(s, "://")
		if schemeEnd > 0 {
			dialectSuffix = s[idx:schemeEnd] // e.g. "+asyncpg"
			s = s[:idx] + s[schemeEnd:]      // strip dialect suffix for parsing
		}
	}

	u, err := url.Parse(s)
	if err != nil {
		// Not a URL — return unchanged.
		return raw, nil
	}
	if u.Host == "" {
		// Relative or non-network URL — don't touch it.
		return raw, nil
	}

	// Path is "/mydb" — replace the database name component.
	parts := strings.SplitN(u.Path, "/", 3)
	switch len(parts) {
	case 0, 1:
		// No database name present; append it.
		u.Path = "/" + tenantDB
	case 2:
		// "/mydb" → "/tenantDB"
		u.Path = "/" + tenantDB
	default:
		// "/mydb/extra" — preserve anything after the db name.
		u.Path = "/" + tenantDB + "/" + parts[2]
	}

	rewritten := u.String()

	// Re-add dialect suffix after scheme.
	if dialectSuffix != "" {
		schemeEnd := strings.Index(rewritten, "://")
		if schemeEnd > 0 {
			rewritten = rewritten[:schemeEnd] + dialectSuffix + rewritten[schemeEnd:]
		}
	}

	return jdbcPrefix + rewritten, nil
}

// RewriteURLEnvs rewrites the path component of each env var listed in
// urlEnvs to use tenantDB, leaving all other env vars unchanged.
// Returns a new map; the original is not modified.
func RewriteURLEnvs(envs map[string]string, urlEnvs []string, tenantDB string) (map[string]string, error) {
	if len(urlEnvs) == 0 || tenantDB == "" {
		return envs, nil
	}
	rewriteSet := make(map[string]bool, len(urlEnvs))
	for _, k := range urlEnvs {
		rewriteSet[k] = true
	}
	out := make(map[string]string, len(envs))
	var errs []string
	for k, v := range envs {
		if rewriteSet[k] {
			rewritten, err := RewriteURL(v, tenantDB)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", k, err))
				out[k] = v
				continue
			}
			out[k] = rewritten
		} else {
			out[k] = v
		}
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("url rewrite errors: %s", strings.Join(errs, "; "))
	}
	return out, nil
}
