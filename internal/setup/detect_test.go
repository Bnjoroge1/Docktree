package setup

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/bnjoroge/docktree/internal/compose"
)

func TestClassifyImage(t *testing.T) {
	cases := []struct {
		image string
		want  string
	}{
		// straight matches with various tag styles
		{"postgres:15-alpine", "postgres"},
		{"postgres", "postgres"},
		{"redis:7-alpine", "redis"},
		{"mysql:8", "mysql"},
		{"mariadb:11", "mysql"},
		{"mongo:7", "mongodb"},
		{"minio:latest", "s3"},

		// org/registry prefixes get stripped before matching
		{"library/postgres:16", "postgres"},
		{"ghcr.io/cloudnative-pg/postgresql:16", "postgres"},
		{"docker.io/library/redis:7", "redis"},
		{"timescale/timescaledb:latest-pg15", "postgres"},
		{"eqalpha/keydb:latest", "redis"},
		{"docker.dragonflydb.io/dragonflydb/dragonfly:latest", "redis"},
		{"valkey/valkey:7", "redis"},
		{"percona/percona-server-mongodb:6", "mongodb"},
		{"percona/percona-server-mysql:8", "mysql"},
		{"percona:5.7", "mysql"},
		{"localstack/localstack:3", "s3"},

		// private registries with a port — the older strip order truncated
		// at the registry's colon. These pin the post-fix order.
		{"localhost:5000/library/postgres:16", "postgres"},
		{"registry.internal:8443/team/redis:7-alpine", "redis"},
		{"127.0.0.1:5000/mongo:7", "mongodb"},

		// digests
		{"postgres@sha256:abcdef", "postgres"},

		// excluded sidecars/admin/monitoring images — must NOT classify
		// despite containing a kind token (postgres, redis, mongo, etc.).
		{"mongo-express:latest", ""},
		{"ghcr.io/mongo-express/mongo-express:1.0", ""},
		{"redis-commander:latest", ""},
		{"redisinsight:2.0", ""},
		{"dpage/pgadmin4:latest", ""},
		{"adminer:latest", ""},
		{"phppgadmin:latest", ""},
		{"phpmyadmin:latest", ""},
		{"postgres-exporter:latest", ""},
		{"bitnami/postgresql-exporter:16", ""},
		{"prom/prometheus-redis-exporter:v1", ""},
		{"backrest:latest", ""},

		// generic exporter suffix — the -exporter/_exporter suffix check
		// catches any Prometheus-style exporter without enumerating each.
		{"mysqld-exporter:latest", ""},
		{"mysql-exporter:v0.15", ""},
		{"mongodb-exporter:latest", ""},
		{"mongodb_exporter:latest", ""},
		{"redis_exporter:latest", ""},
		{"ghcr.io/oliver006/redis_exporter:v1", ""},

		// excluded sidecars with digests — stripTag must handle @sha256:...
		// without leaving a residual "postgres" suffix from the digest colon.
		{"postgres-exporter@sha256:abcdef", ""},
		{"mongo-express@sha256:123456", ""},
		{"bitnami/postgresql-exporter@sha256:deadbeef", ""},

		// excluded sidecars with registry prefixes — stripRegistry must
		// normalize "docker.io/bitnami/..." to "bitnami/..." before
		// checking excludedFullpathSet.
		{"docker.io/bitnami/postgresql-exporter:latest", ""},
		{"docker.io/library/mongo-express:latest", ""},
		{"localhost:5000/library/redis-commander:latest", ""},
		{"registry.internal:8443/bitnami/postgresql-exporter:16", ""},
		// not shareable — application/runtime images must never match
		{"my-app:latest", ""},
		{"nginx:alpine", ""},
		{"caddy:2-alpine", ""},
		{"python:3.12-slim", ""},
		{"node:20", ""},
		{"ghcr.io/myorg/api:v1", ""},
		{"prom/prometheus:latest", ""},
		{"nicolaka/netshoot:latest", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.image, func(t *testing.T) {
			if got := classifyImage(c.image); got != c.want {
				t.Errorf("classifyImage(%q) = %q, want %q", c.image, got, c.want)
			}
		})
	}
}


func TestDetectConsumerURLEnvs(t *testing.T) {
	// Use a candidate name ("postgres") that is not a substring of any
	// unrelated value in the fixture, so the test isolates the heuristic
	// from the substring-collision behavior covered separately below.
	services := map[string]compose.Service{
		"postgres": {
			Image: "postgres:15-alpine",
			Environment: map[string]string{
				// Candidate's own env — must NEVER appear in result.
				"POSTGRES_DB":   "app",
				"POSTGRES_USER": "app",
			},
		},
		"api": {
			Image: "my-api:latest",
			Environment: map[string]string{
				"DATABASE_URL": "postgres://postgres:5432/app", // include: key matches, value references postgres
				"REDIS_URL":    "redis://cache:6379",           // exclude: doesn't reference postgres
				"LOG_PREFIX":   "postgres-events",              // exclude: key doesn't match urlEnvPattern
				"DB_DSN":       "postgresql://postgres/app",    // include: key matches (_DSN), value references postgres
				"NODE_ENV":     "production",
			},
		},
		"worker": {
			Image: "my-worker:latest",
			Environment: map[string]string{
				"DATABASE_URL": "postgres://postgres:5432/app", // dedup with api's
				"MONGODB_URI":  "mongodb://mongo/x",            // exclude: doesn't reference postgres
			},
		},
	}
	got := detectConsumerURLEnvs("postgres", services)
	want := []string{"DATABASE_URL", "DB_DSN"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("detectConsumerURLEnvs(postgres) mismatch\ngot:  %v\nwant: %v", got, want)
	}

	// No candidate or empty services -> nil.
	if got := detectConsumerURLEnvs("", services); got != nil {
		t.Errorf("empty candidate should yield nil, got %v", got)
	}
	if got := detectConsumerURLEnvs("postgres", nil); got != nil {
		t.Errorf("nil services should yield nil, got %v", got)
	}
	// No consumer references the candidate -> nil (not empty slice).
	bare := map[string]compose.Service{
		"postgres": {Image: "postgres:15"},
		"api":      {Image: "my-api", Environment: map[string]string{"NODE_ENV": "production"}},
	}
	if got := detectConsumerURLEnvs("postgres", bare); got != nil {
		t.Errorf("no consumer refs should yield nil, got %v", got)
	}
}

func TestDetectConsumerURLEnvsExcludesNonURLKeys(t *testing.T) {
	// A consumer service references the candidate via many env keys, but
	// only values that are parseable connection URLs belong in url_envs.
	// POSTGRES_HOST/USER/PASSWORD/DB are NOT URLs — the runtime's URL
	// rewriter would no-op on them, silently breaking per_database
	// isolation. Only DATABASE_URL should survive.
	services := map[string]compose.Service{
		"db": {Image: "postgres:15"},
		"api": {
			Image: "my-api",
			Environment: map[string]string{
				"POSTGRES_HOST":     "db",                          // exclude: not _URL/_URI/_DSN
				"POSTGRES_USER":     "app",                         // exclude
				"POSTGRES_PASSWORD": "secret",                      // exclude
				"POSTGRES_DB":       "app",                         // exclude (belongs in db_name_envs)
				"DATABASE_URL":      "postgres://app:secret@db/app", // include
			},
		},
	}
	got := detectConsumerURLEnvs("db", services)
	want := []string{"DATABASE_URL"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("detectConsumerURLEnvs mismatch\ngot:  %v\nwant: %v", got, want)
	}
}

func TestReferencesService(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		candidate string
		want      bool
	}{
		// URL path: hostname must equal candidate exactly.
		{"url hostname exact", "postgres://db:5432/app", "db", true},
		{"url hostname suffix mismatch", "postgres://db.internal:5432/app", "db", false},
		{"url different host", "mongodb://other/x", "db", false},
		{"url userinfo not host", "postgres://db@host:5432/x", "db", false},
		{"url userinfo plus matching host", "postgres://app:secret@db:5432/x", "db", true},

		// Opaque URI (jdbc:...) has no Host — falls through to regex.
		{"jdbc opaque matches via regex", "jdbc:postgresql://db:5432/x", "db", true},

		// Key=value forms have no scheme/host — regex path.
		{"host kv pair", "host=db port=5432", "db", true},
		{"underscore is identifier, not boundary", "host=db_old port=5432", "db", false},
		{"comma-separated host list", "primary,db,replica", "db", true},
		{"longer hostname containing candidate", "host=mydb port=5432", "db", false},

		// Edge cases.
		{"empty value", "", "db", false},
		{"candidate with regex metachar (literal)", "host=a.b port=5432", "a.b", true},
		{"candidate with regex metachar absent", "host=axb", "a.b", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Compile the token-boundary pattern the same way
			// detectConsumerURLEnvs does. Coupled deliberately: if the
			// production regex changes, this line must change too.
			pat := regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(c.candidate) + `([^A-Za-z0-9_-]|$)`)
			if got := referencesService(c.value, c.candidate, pat); got != c.want {
				t.Errorf("referencesService(%q, %q) = %v, want %v", c.value, c.candidate, got, c.want)
			}
		})
	}
}

func TestDetectShareable(t *testing.T) {
	project := &compose.ComposeProject{
		Services: map[string]compose.Service{
			"db": {
				Image: "postgres:15-alpine",
				Environment: map[string]string{
					"POSTGRES_DB":   "app",
					"POSTGRES_USER": "app",
				},
			},
			"cache": {
				Image: "redis:7-alpine",
			},
			"api": {
				Image: "my-app:latest",
				Environment: map[string]string{
					"DATABASE_URL": "postgres://db/app",
				},
			},
			"web": {
				Image: "nginx:alpine",
			},
		},
	}
	got := DetectShareable(project)
	want := []ServiceCandidate{
		{ServiceName: "cache", Kind: "redis", Image: "redis:7-alpine"},
		{ServiceName: "db", Kind: "postgres", Image: "postgres:15-alpine", URLEnvs: []string{"DATABASE_URL"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DetectShareable mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestDetectShareableEmpty(t *testing.T) {
	if got := DetectShareable(nil); got != nil {
		t.Errorf("nil project should yield nil, got %v", got)
	}
	if got := DetectShareable(&compose.ComposeProject{}); got != nil {
		t.Errorf("empty project should yield nil, got %v", got)
	}
	onlyApp := &compose.ComposeProject{
		Services: map[string]compose.Service{
			"web": {Image: "nginx:alpine"},
			"api": {Image: "my-app:latest"},
		},
	}
	// DetectShareable returns an empty (non-nil) slice when scanned but no
	// matches found. Callers test len(out) == 0, not == nil.
	got := DetectShareable(onlyApp)
	if len(got) != 0 {
		t.Errorf("no shareable services should yield empty slice, got %v", got)
	}
}

func TestHintLine(t *testing.T) {
	if got := HintLine(nil); got != "" {
		t.Errorf("nil candidates should yield empty hint, got %q", got)
	}
	got := HintLine([]ServiceCandidate{
		{ServiceName: "db", Kind: "postgres"},
		{ServiceName: "cache", Kind: "redis"},
		{ServiceName: "db2", Kind: "postgres"}, // dedup by kind
	})
	want := "tip: detected shareable services (postgres, redis). " +
		"Ask your AI agent to set up a shared platform tier in docktree.yml."
	if got != want {
		t.Errorf("HintLine mismatch\ngot:  %q\nwant: %q", got, want)
	}
}
