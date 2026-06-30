package compose

import (
	"strings"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
)

func TestFilterServicesRemovesSkipped(t *testing.T) {
	raw := makeTestProject()
	filtered, err := FilterServices(raw, ServiceFilter{Skip: []string{"ui", "caddy"}})
	if err != nil {
		t.Fatalf("FilterServices error: %v", err)
	}
	for _, name := range []string{"ui", "caddy"} {
		if _, ok := filtered.Services[name]; ok {
			t.Errorf("expected service %q to be removed", name)
		}
	}
	for _, name := range []string{"api", "migrate", "infisical", "db"} {
		if _, ok := filtered.Services[name]; !ok {
			t.Errorf("expected service %q to be kept", name)
		}
	}
}

func TestFilterServicesPrunesDependsOn(t *testing.T) {
	raw := makeTestProject()
	filtered, err := FilterServices(raw, ServiceFilter{Skip: []string{"infisical"}})
	if err != nil {
		t.Fatalf("FilterServices error: %v", err)
	}
	api := filtered.Services["api"]
	if _, ok := api.DependsOn["infisical"]; ok {
		t.Errorf("expected api depends_on infisical to be pruned")
	}
	if _, ok := api.DependsOn["db"]; !ok {
		t.Errorf("expected api depends_on db to be kept")
	}
	migrate := filtered.Services["migrate"]
	if len(migrate.DependsOn) != 0 {
		t.Errorf("expected migrate depends_on to be empty, got %v", serviceDeps(migrate))
	}
}

func TestFilterServicesDropDependencies(t *testing.T) {
	raw := makeTestProject()
	filtered, err := FilterServices(raw, ServiceFilter{DropDeps: []string{"infisical"}})
	if err != nil {
		t.Fatalf("FilterServices error: %v", err)
	}
	if _, ok := filtered.Services["infisical"]; !ok {
		t.Fatalf("expected infisical to be kept when only dropping dependencies")
	}
	api := filtered.Services["api"]
	if _, ok := api.DependsOn["infisical"]; ok {
		t.Errorf("expected api depends_on infisical to be pruned")
	}
	if _, ok := api.DependsOn["db"]; !ok {
		t.Errorf("expected api depends_on db to be kept")
	}
}

func TestFilterServicesSkipImpliesDrop(t *testing.T) {
	raw := makeTestProject()
	filtered, err := FilterServices(raw, ServiceFilter{Skip: []string{"ui"}})
	if err != nil {
		t.Fatalf("FilterServices error: %v", err)
	}
	caddy := filtered.Services["caddy"]
	if _, ok := caddy.DependsOn["ui"]; ok {
		t.Errorf("expected caddy depends_on ui to be pruned because ui is skipped")
	}
	if _, ok := caddy.DependsOn["api"]; !ok {
		t.Errorf("expected caddy depends_on api to be kept")
	}
}

func TestFilterServicesUnknownServiceError(t *testing.T) {
	raw := makeTestProject()
	_, err := FilterServices(raw, ServiceFilter{Skip: []string{"missing"}})
	if err == nil {
		t.Fatalf("expected error for unknown skip service")
	}
	if !strings.Contains(err.Error(), "skip_services") {
		t.Errorf("expected error to mention skip_services, got %v", err)
	}

	_, err = FilterServices(raw, ServiceFilter{DropDeps: []string{"ghost"}})
	if err == nil {
		t.Fatalf("expected error for unknown drop dependency")
	}
	if !strings.Contains(err.Error(), "drop_dependencies") {
		t.Errorf("expected error to mention drop_dependencies, got %v", err)
	}
}

func TestFilterServicesNoOpWhenEmpty(t *testing.T) {
	raw := makeTestProject()
	filtered, err := FilterServices(raw, ServiceFilter{})
	if err != nil {
		t.Fatalf("FilterServices error: %v", err)
	}
	if len(filtered.Services) != len(raw.Services) {
		t.Fatalf("expected same service count, got %d want %d", len(filtered.Services), len(raw.Services))
	}
	for name, svc := range raw.Services {
		if got := filtered.Services[name]; got.Name != svc.Name {
			t.Errorf("expected service %q unchanged", name)
		}
	}
}

func TestFilterServicesPreservesProfiles(t *testing.T) {
	raw := makeTestProject()
	filtered, err := FilterServices(raw, ServiceFilter{Skip: []string{"ui"}})
	if err != nil {
		t.Fatalf("FilterServices error: %v", err)
	}
	api := filtered.Services["api"]
	if len(api.Profiles) != 1 || api.Profiles[0] != "api" {
		t.Errorf("expected api profiles preserved, got %v", api.Profiles)
	}
}

func TestFilterServicesString(t *testing.T) {
	f := ServiceFilter{Skip: []string{"caddy", "ui"}, DropDeps: []string{"infisical"}}
	got := f.String()
	want := "skip=caddy,ui drop_deps=infisical"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	empty := ServiceFilter{}
	if empty.String() != "" {
		t.Errorf("empty filter String() should be empty")
	}
}

func makeTestProject() *composetypes.Project {
	return &composetypes.Project{
		Name: "test",
		Services: composetypes.Services{
			"api": {
				Name: "api",
				DependsOn: composetypes.DependsOnConfig{
					"db":        {Condition: composetypes.ServiceConditionHealthy},
					"infisical": {Condition: composetypes.ServiceConditionStarted},
				},
				Profiles: []string{"api"},
			},
			"migrate": {
				Name: "migrate",
				DependsOn: composetypes.DependsOnConfig{
					"infisical": {Condition: composetypes.ServiceConditionCompletedSuccessfully},
				},
			},
			"ui": {
				Name: "ui",
				DependsOn: composetypes.DependsOnConfig{
					"api":   {Condition: composetypes.ServiceConditionStarted},
					"caddy": {Condition: composetypes.ServiceConditionStarted},
				},
			},
			"caddy": {
				Name: "caddy",
				DependsOn: composetypes.DependsOnConfig{
					"api": {Condition: composetypes.ServiceConditionStarted},
					"ui":  {Condition: composetypes.ServiceConditionStarted},
				},
			},
			"infisical": {Name: "infisical"},
			"db":        {Name: "db"},
		},
	}
}

func serviceDeps(svc composetypes.ServiceConfig) []string {
	out := make([]string, 0, len(svc.DependsOn))
	for name := range svc.DependsOn {
		out = append(out, name)
	}
	return out
}
