package setup

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/bnjoroge/docktree/internal/compose"
)

type ServiceCandidate struct {
	ServiceName string   `json:"service"`
	Kind        string   `json:"kind"`
	Image       string   `json:"image"`
	// URLEnvs are env var keys on *consumer* services (not the candidate)
	// whose value references the candidate. Heuristic — agent confirms with user.
	URLEnvs []string `json:"url_envs,omitempty"`
}

// Order matters: first match wins. MongoDB before mysql because
// "percona-server-mongodb" contains both "mongo" and "percona".
var kindPatterns = []struct {
	kind     string
	patterns []string
}{
	{"postgres", []string{"postgres", "postgis", "timescaledb", "pgvector"}},
	{"mongodb", []string{"mongo"}},
	{"mysql", []string{"mysql", "mariadb", "percona"}},
	{"redis", []string{"redis", "keydb", "dragonfly", "valkey"}},
	{"s3", []string{"minio", "localstack", "seaweedfs"}},
}

// False-positive (misclassify sidecar as shareable) is worse than
// false-negative (user adds it manually). Bare basenames match any
// registry prefix; org/basename entries match the full image path.
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
	"bitnami/postgresql-exporter",
}

var excludedBasenameSet map[string]struct{}
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

// Intentionally narrow to _URL/_URI/_DSN: keys like POSTGRES_HOST or
// POSTGRES_DB reference a shared service but their values are not URLs,
// so putting them in url_envs would make RewriteURL a no-op and give
// false isolation confidence.
var urlEnvPattern = regexp.MustCompile(`(?i)(_URL|_URI|_DSN)$`)

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

// Registry ports (localhost:5000/...) must not be confused with image tags
// (postgres:16) — stripImageTag operates on the path component after the
// last '/' only.
func classifyImage(image string) string {
	if image == "" {
		return ""
	}
	lower := strings.ToLower(image)
	// Normalize registry prefix so "docker.io/bitnami/..." matches the
	// exclusion entry "bitnami/...".
	fullPath := stripRegistry(stripImageTag(lower))
	if _, ok := excludedFullpathSet[fullPath]; ok {
		return ""
	}
	ref := fullPath
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	if _, ok := excludedBasenameSet[ref]; ok {
		return ""
	}
	// Suffix check catches any Prometheus-style exporter without
	// enumerating every permutation (mysqld-exporter, redis_exporter, ...).
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

func stripImageTag(image string) string {
	if i := strings.LastIndex(image, "/"); i >= 0 {
		base := image[i+1:]
		base = stripTag(base)
		return image[:i+1] + base
	}
	return stripTag(image)
}

// Same heuristic Docker uses to distinguish a registry from an org namespace.
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

// Digest (@sha256:...) takes precedence over tag (:16): if '@' exists,
// cut there; otherwise cut at the last ':'.
func stripTag(segment string) string {
	if at := strings.LastIndex(segment, "@"); at >= 0 {
		return segment[:at]
	}
	if colon := strings.LastIndex(segment, ":"); colon >= 0 {
		return segment[:colon]
	}
	return segment
}

// Scans *other* services for env keys whose value references the candidate.
// We don't scan the candidate's own env because url_envs names vars on
// consumer services that the runtime rewrites per worktree — surfacing
// the candidate's own POSTGRES_DB here would silently break isolation.
func detectConsumerURLEnvs(candidate string, services map[string]compose.Service) []string {
	if candidate == "" || len(services) == 0 {
		return nil
	}
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

// Two phases: URL values get exact hostname matching (so mongodb://other
// doesn't match "db"); non-URL values fall back to token-boundary regex
// (so host=db matches but db_old doesn't).
func referencesService(value, candidate string, refPattern *regexp.Regexp) bool {
	if value == "" {
		return false
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Hostname() == candidate
	}
	return refPattern.MatchString(value)
}

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
