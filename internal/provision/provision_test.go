package provision

import (
	"strings"
	"testing"
)

func TestTenantName(t *testing.T) {
	cases := []struct {
		repo, instance, want string
	}{
		{"myrepo", "myrepo-main-abc123", "myrepo_myrepo_main_abc123"},
		{"cow-shared-services", "cow-main-a1b2c3", "cow_shared_services_cow_main_a1b2c3"},
		{"Repo", "Branch-UPPER", "repo_branch_upper"},
		{"r", strings.Repeat("x", 70), "r_" + strings.Repeat("x", 61)},
	}
	for _, tc := range cases {
		got := TenantName(tc.repo, tc.instance)
		if got != tc.want {
			t.Errorf("TenantName(%q, %q) = %q, want %q", tc.repo, tc.instance, got, tc.want)
		}
		if len(got) > 63 {
			t.Errorf("TenantName exceeds 63 bytes: len=%d", len(got))
		}
	}
}

func TestTenantNameNeverEmpty(t *testing.T) {
	got := TenantName("", "")
	if got == "" {
		t.Fatal("TenantName should never return empty string")
	}
}

func TestProvisionFullSharePassesThroughToDriver(t *testing.T) {
	err := Provision(TenantConfig{
		Kind:    "postgres",
		Tenancy: "full_share",
	})
	if err == nil {
		t.Fatal("full_share without a tenant name should return an error from the driver")
	}
	if !strings.Contains(err.Error(), "empty tenant name") {
		t.Fatalf("expected 'empty tenant name' error, got: %v", err)
	}
}

func TestProvisionUnknownKindErrors(t *testing.T) {
	err := Provision(TenantConfig{Kind: "neo4j", Tenancy: "per_database"})
	if err == nil {
		t.Fatal("unknown kind should return error")
	}
	if !strings.Contains(err.Error(), "neo4j") {
		t.Fatalf("error should mention kind, got: %v", err)
	}
}

func TestProvisionRedisNoOp(t *testing.T) {
	err := Provision(TenantConfig{Kind: "redis", Tenancy: "full_share"})
	if err != nil {
		t.Fatalf("redis should be no-op: %v", err)
	}
}

func TestTenantDriversRegistered(t *testing.T) {
	for _, kind := range []string{"postgres", "mysql", "mongodb"} {
		if _, ok := tenantDrivers[kind]; !ok {
			t.Fatalf("expected %s tenant driver to be registered", kind)
		}
	}
}

func TestMySQLIdentifierEscaping(t *testing.T) {
	got := escapeMySQLIdentifier("tenant`db")
	if got != "tenant``db" {
		t.Fatalf("escapeMySQLIdentifier = %q, want %q", got, "tenant``db")
	}
}

func TestSQLStringEscaping(t *testing.T) {
	got := escapeSQLString("tenant'db")
	if got != "tenant''db" {
		t.Fatalf("escapeSQLString = %q, want %q", got, "tenant''db")
	}
}

func TestEscapePostgresIdentifier(t *testing.T) {
	cases := []struct{ input, want string }{
		{"app", "app"},
		{"my\"db", "my\"\"db"},
		{"\"quoted\"", "\"\"quoted\"\""},
	}
	for _, tc := range cases {
		if got := escapePostgresIdentifier(tc.input); got != tc.want {
			t.Errorf("escapePostgresIdentifier(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestEscapePostgresLiteral(t *testing.T) {
	cases := []struct{ input, want string }{
		{"app", "app"},
		{"tenant'db", "tenant''db"},
		{"it''s", "it''''s"},
	}
	for _, tc := range cases {
		if got := escapePostgresLiteral(tc.input); got != tc.want {
			t.Errorf("escapePostgresLiteral(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPostgresDollarQuote(t *testing.T) {
	got := postgresDollarQuote(`tenant\'db`)
	if !strings.HasPrefix(got, "$tenant$") || !strings.HasSuffix(got, `$tenant$`) {
		t.Fatalf("unexpected quote wrapper: %q", got)
	}
	if !strings.Contains(got, `tenant\'db`) {
		t.Fatalf("quoted value missing original content: %q", got)
	}
}

func TestPostgresDollarQuoteRetagsWhenNeeded(t *testing.T) {
	value := `abc$tenant$xyz`
	got := postgresDollarQuote(value)
	if got == "$tenant$"+value+"$tenant$" {
		t.Fatalf("expected alternate tag when value contains default tag: %q", got)
	}
	if !strings.Contains(got, value) {
		t.Fatalf("quoted value missing original content: %q", got)
	}
}

func TestQuoteJSString(t *testing.T) {
	got := quoteJSString(`tenant"db\name`)
	if got != `"tenant\"db\\name"` {
		t.Fatalf("quoteJSString = %q", got)
	}
}

func TestDeprovisionFullShareNoOp(t *testing.T) {
	err := Deprovision(TenantConfig{Kind: "postgres", Tenancy: "full_share"})
	if err != nil {
		t.Fatalf("full_share deprovision should be no-op: %v", err)
	}
}

func TestDeprovisionRedisNoOp(t *testing.T) {
	err := Deprovision(TenantConfig{Kind: "redis", Tenancy: "full_share"})
	if err != nil {
		t.Fatalf("redis deprovision should be no-op: %v", err)
	}
}

func TestDeprovisionUnknownKindErrors(t *testing.T) {
	err := Deprovision(TenantConfig{Kind: "neo4j", Tenancy: "per_database"})
	if err == nil || !strings.Contains(err.Error(), "neo4j") {
		t.Fatalf("expected error mentioning kind, got: %v", err)
	}
}

func TestTenantNameForDatabaseAddsLogicalSuffix(t *testing.T) {
	got := TenantNameForDatabase("docktree", "feature-main-123abc", "infisical")
	if got != "infisical_docktree_feature_main_123abc" {
		t.Fatalf("TenantNameForDatabase = %q", got)
	}
}

func TestTenantNameForDatabaseLogicalSuffixSurvivesTruncation(t *testing.T) {
	long := strings.Repeat("x", 70)
	a := TenantNameForDatabase("r", long, "app")
	b := TenantNameForDatabase("r", long, "infisical")
	if a == b {
		t.Fatalf("logical DBs collided after truncation: %q", a)
	}
	if len(a) > 63 || len(b) > 63 {
		t.Fatalf("exceeds 63 bytes: a=%d b=%d", len(a), len(b))
	}
}

func TestTenantNameForDatabaseLegacyParityWithTenantName(t *testing.T) {
	if got, want := TenantNameForDatabase("repo", "branch-abc123", ""), TenantName("repo", "branch-abc123"); got != want {
		t.Fatalf("legacy parity broken: %q vs %q", got, want)
	}
}

func TestSharedTenantNameStripsInstance(t *testing.T) {
	// Two different instances should resolve to the same name for a shared database.
	a := SharedTenantName("myrepo", "infisical")
	b := SharedTenantName("myrepo", "infisical")
	if a != b {
		t.Fatalf("SharedTenantName not stable: %q vs %q", a, b)
	}
	if a != "infisical_myrepo" {
		t.Fatalf("SharedTenantName = %q, want %q", a, "infisical_myrepo")
	}
}

func TestSharedTenantNameNoLogicalDB(t *testing.T) {
	got := SharedTenantName("myrepo", "")
	if got != "myrepo" {
		t.Fatalf("SharedTenantName = %q, want %q", got, "myrepo")
	}
}

func TestResolveTenantNamePerDatabase(t *testing.T) {
	got := ResolveTenantName("per_database", "repo", "feat-abc123", "app")
	want := TenantNameForDatabase("repo", "feat-abc123", "app")
	if got != want {
		t.Fatalf("ResolveTenantName per_database = %q, want %q", got, want)
	}
}

func TestResolveTenantNameFullShare(t *testing.T) {
	a := ResolveTenantName("full_share", "repo", "feat-abc123", "infisical")
	b := ResolveTenantName("full_share", "repo", "other-branch-xyz", "infisical")
	if a != b {
		t.Fatalf("full_share should be instance-independent: %q vs %q", a, b)
	}
	want := SharedTenantName("repo", "infisical")
	if a != want {
		t.Fatalf("ResolveTenantName full_share = %q, want %q", a, want)
	}
}
