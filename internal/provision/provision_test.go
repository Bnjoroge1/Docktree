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

func TestProvisionFullShareNoOp(t *testing.T) {
	// full_share tenancy must be a no-op; no docker calls happen.
	err := Provision(TenantConfig{
		Kind:    "postgres",
		Tenancy: "full_share",
	})
	if err != nil {
		t.Fatalf("full_share should be a no-op: %v", err)
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
