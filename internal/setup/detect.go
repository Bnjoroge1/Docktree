package setup

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/bnjoroge/docktree/internal/compose"
)

// ServiceCandidate is a compose service whose image suggests it would benefit
// from running in Docktree's shared platform tier instead of being duplicated
// per worktree (e.g. postgres, redis).
// The agent skill 'docktree-init' consumes these to identify which compose
// services could run in the shared platform tier; 'docktree up' uses them
// to print a one-line tip when the user has no shared services configured
// yet.
type ServiceCandidate struct {
	// ServiceName is the key under compose `services:` (e.g. "db", "cache").
	ServiceName string `json:"service"`
	// Kind is one of: postgres, mysql, mongodb, redis, s3. These match the
	// values config.SharedService.Kind accepts (see allowedTenancyByKind in
	// internal/config/config.go). "generic" is intentionally excluded — it
	// isn't auto-detectable from an image name.
	Kind string `json:"kind"`
	// Image is the raw image reference from the compose file.
	Image string `json:"image"`
	// URLEnvs lists env var KEYS on *consumer* services (anything other than
	// the candidate itself) whose value references the candidate service name
	// AND whose key looks like a connection-string env. These are the env
	// vars the runtime would rewrite under tenancy:per_database. Heuristic,
	// not authoritative — the agent should confirm with the user.
	URLEnvs []string `json:"url_envs,omitempty"`
}

// kindPatterns maps a Kind to substrings that, when found in an image name
// (case-insensitive, after stripping any registry/org prefix), indicate that
// kind. Order matters: the first kind whose pattern list matches wins, so
// more specific kinds must precede generic-token kinds.
var kindPatterns = []struct {
	kind     string
	patterns []string
}{
	// Drop-in postgres replacements share the wire protocol and reuse postgres
	// tenant provisioning, so we classify them as "postgres".
	{"postgres", []string{"postgres", "postgis", "timescaledb", "pgvector"}},
	// MongoDB before mysql: "percona-server-mongodb" contains both "mongo"
	// and "percona". The mongo token is exclusive to MongoDB across all real
	// images, so putting mongodb first gives the correct kind for that image
	// without breaking any other classification (no postgres/mysql/redis
	// image contains "mongo").
	{"mongodb", []string{"mongo"}},
	// "percona" alone (no mongo) is Percona Server for MySQL.
	{"mysql", []string{"mysql", "mariadb", "percona"}},
	// keydb / dragonflydb / valkey speak the Redis protocol.
	{"redis", []string{"redis", "keydb", "dragonfly", "valkey"}},
	// S3-compatible object stores.
	{"s3", []string{"minio", "localstack", "seaweedfs"}},
}

// excludedBasenameEntries contains well-known sidecar/admin/monitoring image
// basenames that match a kindPatterns token but which are NOT the database
// cache service itself. classifyImage returns "" for any image whose
// basename (or full path for slash-containing entries) is in this set.
//
// Two styles of entry:
//   - bare basename (no slash): "postgres-exporter" matches any registry/org
//     prefix, e.g. ghcr.io/foo/postgres-exporter:latest.
//   - org/basename (has slash): "bitnami/postgresql-exporter" matches the
//     full image path, e.g. bitnami/postgresql-exporter:16 — useful when
//     the same basename under a different org IS a real DB service.
//
// Keep sorted within each group. The false-positive cost of misclassifying
// a sidecar as shareable is worse than a false-negative (which just means
// one service doesn't get suggested; the user can always add it manually).
var excludedBasenameEntries = []string{
	"adminer",
	"backrest",
	"dbeaver-cloudbeaver",
	"mongo-express",
	"mongoclient",
	"pgadmin",
	"pgadmin4",
	"phppgadmin",
	"phpmyadmin",
	"redis-commander",
	"redisinsight",
	"redmon",
	// Full org/path entries — match against full lowercased image path.
	"bitnami/postgresql-exporter",
}

// excludedBasenameSet is built once at init from excludedBasenameEntries for
// O(1) lookup. Only bare basenames (no slash) are included — classifyImage
// checks these against the registry-stripped basename.
var excludedBasenameSet map[string]struct{}

// excludedFullpathSet contains entries that include an org prefix (contain a
// slash). classifyImage checks these against the full lowercased image ref
// (tag stripped but prefix kept).
var excludedFullpathSet map[string]struct{}

func init() {
	excludedBasenameSet = make(map[string]struct{}, len(excludedBasenameEntries))
	excludedFullpathSet = make(map[string]struct{})
	for _, e := range excludedBasenameEntries {
		if strings.Contains(e, "/") {
			excludedFullpathSet[e] = struct{}{}
		} else {
			excludedBasenameSet[e] = struct{}{}
		}
	}
}

// urlEnvPattern matches env var KEYS that conventionally hold a parseable
// connection URL/URI/DSN — the only shapes the runtime's URL rewriter knows
// how to mutate per worktree.
//
// Intentionally narrow: keys like POSTGRES_HOST, POSTGRES_USER, POSTGRES_DB,
// REDIS_PASSWORD reference a shared service but their VALUES are not URLs,
// so feeding them into shared.services.*.url_envs would make RewriteURL a
// no-op and give false isolation confidence (POSTGRES_DB belongs under the
// separate db_name_envs field, not url_envs).
var urlEnvPattern = regexp.MustCompile(`(?i)(_URL|_URI|_DSN)$`)

// DetectShareable scans a loaded compose project and returns services whose
// image matches a known shareable-service pattern, with detected connection
// env var names. Returns candidates sorted by service name for stable output.
//
// Pure function: no I/O.
func DetectShareable(project *compose.ComposeProject) []ServiceCandidate {
	if project == nil || len(project.Services) == 0 {
		return nil
	}
	out := make([]ServiceCandidate, 0)
	for name, svc := range project.Services {
		kind := classifyImage(svc.Image)
		if kind == "" {
			continue
		}
		out = append(out, ServiceCandidate{
			ServiceName: name,
			Kind:        kind,
			Image:       svc.Image,
			URLEnvs:     detectConsumerURLEnvs(name, project.Services),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServiceName < out[j].ServiceName })
	return out
}

// classifyImage returns the Kind for an image reference, or "" if no pattern
// matches, or if the image is in excludedBasenameSet/excludedFullpathSet.
//
// Registry ports (localhost:5000/...) must not be confused with image tags
// (postgres:16). The helper stripImageTag operates on the path component
// after the last '/' only, so registry colons are never truncated.
func classifyImage(image string) string {
	if image == "" {
		return ""
	}
	lower := strings.ToLower(image)
	// Check full-path exclusions BEFORE stripping the registry prefix.
	// Normalize by dropping the optional registry segment so that
	// "docker.io/bitnami/postgresql-exporter" matches the exclusion
	// entry "bitnami/postgresql-exporter".
	fullPath := stripRegistry(stripImageTag(lower))
	if _, ok := excludedFullpathSet[fullPath]; ok {
		return ""
	}
	// Extract the basename by stripping registry/org prefix from the
	// tag-cleaned full path: "ghcr.io/foo/postgres" -> "postgres".
	ref := fullPath
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	// Check bare-basename exclusions against the registry-stripped ref.
	if _, ok := excludedBasenameSet[ref]; ok {
		return ""
	}
	// Any Prometheus-style exporter (mysqld-exporter, redis_exporter,
	// mongodb-exporter, etc.) is a monitoring sidecar, not a database
	// service. Match by suffix so we don't need to enumerate every
	// permutation.
	if strings.HasSuffix(ref, "-exporter") || strings.HasSuffix(ref, "_exporter") {
		return ""
	}
	for _, p := range kindPatterns {
		for _, pat := range p.patterns {
			if strings.Contains(ref, pat) {
				return p.kind
			}
		}
	}
	return ""
}

// stripImageTag removes the tag or digest from a lowercased image reference,
// operating only on the path component after the last '/' so that registry
// ports (localhost:5000/...) are never truncated.
//
//	"postgres:16-alpine"            -> "postgres"
//	"localhost:5000/lib/postgres:16" -> "localhost:5000/lib/postgres"
//	"postgres@sha256:abcdef"        -> "postgres"
//	"ghcr.io/foo/postgres"          -> "ghcr.io/foo/postgres"  (unchanged)
//	"minio"                         -> "minio"
func stripImageTag(image string) string {
	if i := strings.LastIndex(image, "/"); i >= 0 {
		// Has a path: strip tag/digest from the basename only.
		base := image[i+1:]
		base = stripTag(base)
		return image[:i+1] + base
	}
	// No path: strip tag/digest from the whole string.
	return stripTag(image)
}

// stripRegistry drops the leading segment of a slash-separated image path
// if it looks like a Docker registry hostname (contains '.', ':', or is
// "localhost"). This is the same heuristic Docker uses to distinguish a
// registry from an org/user namespace.
//
//	"docker.io/bitnami/postgresql-exporter" -> "bitnami/postgresql-exporter"
//	"ghcr.io/myorg/postgres"                -> "myorg/postgres"
//	"localhost:5000/lib/redis"              -> "lib/redis"
//	"bitnami/postgresql-exporter"           -> "bitnami/postgresql-exporter" (unchanged)
//	"postgres"                              -> "postgres" (unchanged)
func stripRegistry(path string) string {
	i := strings.Index(path, "/")
	if i <= 0 {
		return path
	}
	host := path[:i]
	if host == "localhost" || strings.ContainsAny(host, ".:") {
		return path[i+1:]
	}
	return path
}

// stripTag removes the trailing :tag or @digest from a single path segment
// (no '/' expected). Digest (@sha256:...) takes precedence over tag (:16):
// if '@' exists, cut there; otherwise cut at the last ':'.
func stripTag(segment string) string {
	if at := strings.LastIndex(segment, "@"); at >= 0 {
		return segment[:at]
	}
	if colon := strings.LastIndex(segment, ":"); colon >= 0 {
		return segment[:colon]
	}
	return segment
}

// detectConsumerURLEnvs returns env var KEYS on services other than the
// candidate, where the key looks like a connection-string env AND the value
// references the candidate service name (typically the host segment of a
// connection URL). Sorted, deduplicated.
//
// Why not scan the candidate's own env? `shared.services.<name>.url_envs`
// names env vars on CONSUMER (app) services that the runtime rewrites per
// worktree — not env vars on the database container itself. Surfacing the
// candidate's own POSTGRES_DB/POSTGRES_USER here would cause a generated
// docktree.yml to put the wrong env names under url_envs, which silently
// breaks per_database isolation (the runtime would find no app-side env to
// rewrite, and all worktrees would land on the same database).
func detectConsumerURLEnvs(candidate string, services map[string]compose.Service) []string {
	if candidate == "" || len(services) == 0 {
		return nil
	}
	// Token-boundary fallback for non-URL forms (host=db, jdbc:...://db:port).
	// `_` is treated as part of the identifier so "db_old" does NOT match "db".
	refPattern := regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(candidate) + `([^A-Za-z0-9_-]|$)`)
	seen := make(map[string]struct{})
	for name, svc := range services {
		if name == candidate {
			continue
		}
		for k, v := range svc.Environment {
			if !urlEnvPattern.MatchString(k) {
				continue
			}
			if !referencesService(v, candidate, refPattern) {
				continue
			}
			seen[k] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// referencesService reports whether value plausibly refers to the candidate
// service hostname. Two phases:
//
//  1. If the value parses as a URL with both a scheme and a host
//     (postgres://db:5432/app), require u.Hostname() == candidate exactly.
//     This is the only check that fires for URLs — `mongodb://other/x` and
//     `postgres://db.internal:5432` both correctly fail to match "db".
//
//  2. Otherwise (key=value forms, JDBC-style opaque URIs, comma-separated
//     host lists), fall back to a token-boundary regexp so "host=db",
//     "jdbc:postgresql://db:5432", and ",db," all match while "db_old",
//     "mongodb", and "stub" do not.
func referencesService(value, candidate string, refPattern *regexp.Regexp) bool {
	if value == "" {
		return false
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Hostname() == candidate
	}
	return refPattern.MatchString(value)
}

// HintLine renders a single-line user-facing tip summarizing the candidates,
// or "" if there are none. Used by `docktree up` on first-time runs in a
// worktree that has no shared.services configured.
func HintLine(candidates []ServiceCandidate) string {
	if len(candidates) == 0 {
		return ""
	}
	kinds := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		kinds[c.Kind] = struct{}{}
	}
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	return "tip: detected shareable services (" + strings.Join(names, ", ") +
		"). Ask your AI agent to set up a shared platform tier in docktree.yml."
}
