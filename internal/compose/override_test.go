package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnjoroge/docktree/internal/ports"
)

func TestGenerateOverride(t *testing.T) {
	tests := []struct {
		name          string
		project       *ComposeProject
		instance      string
		assignments   []ports.Assignment
		sharedVolumes []string
		check         func(t *testing.T, override *Override)
		wantErr       bool
	}{
		{
			name: "full_rewrite",
			project: &ComposeProject{Services: map[string]Service{
				"web": {
					ContainerName: "myapp_web",
					Image:         "myapp/web:v1",
					Build:         &BuildConfig{Context: "."},
					Ports:         []PortMapping{{Target: 80, Published: 8080, HostIP: "127.0.0.1", Protocol: "tcp"}},
				},
				"redis": {Image: "redis:7"},
			}},
			instance:    "repo-branch-abcdef",
			assignments: []ports.Assignment{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1", HostPort: 41000}},
			check: func(t *testing.T, o *Override) {
				web := o.Services["web"]
				if web.ContainerName == nil || *web.ContainerName != "repo-branch-abcdef-web" {
					t.Fatalf("container_name: got %#v", web.ContainerName)
				}
				if web.Image != "docktree/repo-branch-abcdef/web:v1" {
					t.Fatalf("image: got %q", web.Image)
				}
				if web.Ports[0].Published != 41000 {
					t.Fatalf("port: got %#v", web.Ports)
				}
				if web.Labels["docktree.managed"] != "true" || web.Labels["docktree.instance"] != "repo-branch-abcdef" {
					t.Fatalf("labels: got %#v", web.Labels)
				}
				redis := o.Services["redis"]
				if redis.Image != "" {
					t.Fatalf("pull-only image should be untouched: %#v", redis)
				}
				if redis.Labels["docktree.managed"] != "true" {
					t.Fatalf("labels missing from pull-only service")
				}
			},
		},
		{
			name: "build_image_tags",
			project: &ComposeProject{Services: map[string]Service{
				"untagged":  {Image: "local/worker", Build: &BuildConfig{Context: "./worker"}},
				"registry":  {Image: "registry.local:5000/team/api:v2", Build: &BuildConfig{Context: "./api"}},
				"buildonly": {Build: &BuildConfig{Context: "./app"}},
			}},
			instance: "repo-feature-abcdef",
			check: func(t *testing.T, o *Override) {
				for svc, want := range map[string]string{
					"untagged":  "docktree/repo-feature-abcdef/untagged:latest",
					"registry":  "docktree/repo-feature-abcdef/registry:v2",
					"buildonly": "docktree/repo-feature-abcdef/buildonly:latest",
				} {
					if got := o.Services[svc].Image; got != want {
						t.Fatalf("%s image = %q, want %q", svc, got, want)
					}
				}
			},
		},
		{
			name: "no_container_name",
			project: &ComposeProject{Services: map[string]Service{
				"web": {Image: "nginx", Ports: []PortMapping{{Target: 80, Published: 8080}}},
			}},
			instance:    "repo-main-abc",
			assignments: []ports.Assignment{{Service: "web", ContainerPort: 80, HostPort: 41000}},
			check: func(t *testing.T, o *Override) {
				if o.Services["web"].ContainerName != nil {
					t.Fatalf("container_name should be nil: got %#v", o.Services["web"].ContainerName)
				}
			},
		},
		{
			name: "no_assignments",
			project: &ComposeProject{Services: map[string]Service{
				"web": {Ports: []PortMapping{{Target: 80, Published: 8080}}},
			}},
			instance: "repo-main-abc",
			check: func(t *testing.T, o *Override) {
				if len(o.Services["web"].Ports) != 0 {
					t.Fatalf("expected no port overrides, got %#v", o.Services["web"].Ports)
				}
			},
		},
		{
			name: "service_without_ports",
			project: &ComposeProject{Services: map[string]Service{
				"worker": {Image: "myapp/worker", Build: &BuildConfig{Context: "."}},
			}},
			instance: "repo-main-abc",
			check: func(t *testing.T, o *Override) {
				if len(o.Services["worker"].Ports) != 0 {
					t.Fatalf("expected no port overrides, got %#v", o.Services["worker"].Ports)
				}
			},
		},
		{
			name: "network_mode_services_skip_network_override",
			project: &ComposeProject{Services: map[string]Service{
				"vpn": {Image: "alpine", NetworkMode: "host"},
			}},
			instance: "repo-main-abc",
			check: func(t *testing.T, o *Override) {
				if networks := o.Services["vpn"].Networks; len(networks) != 0 {
					t.Fatalf("network_mode service should not get networks override, got %#v", networks)
				}
			},
		},
		{
			name:     "nil_project",
			project:  nil,
			instance: "repo-main-abc",
			wantErr:  true,
		},
		{
			name: "isolate_external_volumes",
			project: &ComposeProject{
				Services: map[string]Service{
					"db": {Image: "postgres:16"},
				},
				Volumes: map[string]Volume{
					"db-data":    {External: true},
					"cache-data": {External: true},
				},
			},
			instance:    "repo-feature-abc",
			assignments: nil,
			check: func(t *testing.T, o *Override) {
				if len(o.Volumes) != 2 {
					t.Fatalf("expected 2 volume overrides, got %d", len(o.Volumes))
				}
				dbVol := o.Volumes["db-data"]
				if dbVol.Name != "repo-feature-abc-db-data" {
					t.Fatalf("db-data volume name: got %q", dbVol.Name)
				}
				if dbVol.External == nil || *dbVol.External != false {
					t.Fatalf("db-data external: got %#v", dbVol.External)
				}
				cacheVol := o.Volumes["cache-data"]
				if cacheVol.Name != "repo-feature-abc-cache-data" {
					t.Fatalf("cache-data volume name: got %q", cacheVol.Name)
				}
			},
		},
		{
			name: "shared_volumes_not_overridden",
			project: &ComposeProject{
				Services: map[string]Service{
					"db": {Image: "postgres:16"},
				},
				Volumes: map[string]Volume{
					"db-data":    {External: true},
					"cache-data": {External: true},
				},
			},
			instance:      "repo-feature-abc",
			assignments:   nil,
			sharedVolumes: []string{"cache-data"},
			check: func(t *testing.T, o *Override) {
				if len(o.Volumes) != 1 {
					t.Fatalf("expected 1 volume override (shared volume should be excluded), got %d", len(o.Volumes))
				}
				if _, exists := o.Volumes["cache-data"]; exists {
					t.Fatal("cache-data should not be overridden because it is shared")
				}
				if _, exists := o.Volumes["db-data"]; !exists {
					t.Fatal("db-data should be overridden because it is not shared")
				}
			},
		},
		{
			name: "internal_volumes_not_overridden",
			project: &ComposeProject{
				Services: map[string]Service{
					"db": {Image: "postgres:16"},
				},
				Volumes: map[string]Volume{
					"db-data": {External: false},
				},
			},
			instance:    "repo-feature-abc",
			assignments: nil,
			check: func(t *testing.T, o *Override) {
				if len(o.Volumes) != 0 {
					t.Fatalf("internal volumes should not be overridden, got %d", len(o.Volumes))
				}
			},
		},
		{
			name: "named_internal_volumes_are_isolated",
			project: &ComposeProject{
				Services: map[string]Service{
					"db": {Image: "postgres:16"},
				},
				Volumes: map[string]Volume{
					"db-data": {Name: "shared-db"},
				},
			},
			instance:    "repo-feature-abc",
			assignments: nil,
			check: func(t *testing.T, o *Override) {
				if len(o.Volumes) != 1 {
					t.Fatalf("expected named internal volume override, got %d", len(o.Volumes))
				}
				vol := o.Volumes["db-data"]
				if vol.Name != "repo-feature-abc-db-data" {
					t.Fatalf("db-data volume name: got %q", vol.Name)
				}
				if vol.External == nil || *vol.External != false {
					t.Fatalf("db-data external: got %#v", vol.External)
				}
			},
		},
		{
			name: "shared_named_internal_volumes_not_overridden",
			project: &ComposeProject{
				Services: map[string]Service{
					"db": {Image: "postgres:16"},
				},
				Volumes: map[string]Volume{
					"db-data": {Name: "shared-db"},
				},
			},
			instance:      "repo-feature-abc",
			assignments:   nil,
			sharedVolumes: []string{"db-data"},
			check: func(t *testing.T, o *Override) {
				if len(o.Volumes) != 0 {
					t.Fatalf("shared named internal volume should not be overridden, got %d", len(o.Volumes))
				}
			},
		},
		{
			name: "isolated_network_per_worktree",
			project: &ComposeProject{Services: map[string]Service{
				"api":    {Image: "myapp/api:1"},
				"worker": {Image: "myapp/worker:1"},
			}},
			instance: "repo-feature-abc123",
			check: func(t *testing.T, o *Override) {
				isoNet := "repo-feature-abc123-isolated"
				// Top-level network declaration
				if o.Networks == nil {
					t.Fatal("override.Networks is nil; expected isolated network")
				}
				net, ok := o.Networks[isoNet]
				if !ok {
					t.Fatalf("isolated network %q not declared in override", isoNet)
				}
				if net.Driver != "bridge" {
					t.Fatalf("isolated network driver = %q, want bridge", net.Driver)
				}
				// Every service must reference the isolated network
				for svcName, svc := range o.Services {
					if _, ok := svc.Networks[isoNet]; !ok {
						t.Fatalf("service %q missing isolated network %q", svcName, isoNet)
					}
				}
			},
		},
		{
			name: "namespace_custom_networks",
			project: &ComposeProject{
				Services: map[string]Service{
					"web": {Image: "nginx:latest"},
				},
				Networks: map[string]Network{
					"custom-net":   {Name: "shared_data_net", External: false},
					"external-net": {Name: "already_exists_net", External: true},
					"platform-net": {Name: "docktree-platform-repo-net", External: true},
				},
			},
			instance: "repo-feature-xyz",
			check: func(t *testing.T, o *Override) {
				// custom-net should be rewritten to custom-net-<instance>
				n1, ok := o.Networks["custom-net"]
				if !ok {
					t.Fatal("expected custom-net in override networks")
				}
				if n1.Name != "shared_data_net-repo-feature-xyz" {
					t.Fatalf("expected custom-net Name: shared_data_net-repo-feature-xyz, got %q", n1.Name)
				}
				if n1.External {
					t.Fatal("expected custom-net to not be external")
				}

				// external-net and platform-net should NOT be rewritten
				if _, ok := o.Networks["external-net"]; ok {
					t.Fatal("expected external-net to not be present in override (preserved as external)")
				}
				if _, ok := o.Networks["platform-net"]; ok {
					t.Fatal("expected platform-net to not be present in override (preserved as external)")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			override, err := GenerateOverride(tt.project, tt.instance, tt.assignments, tt.sharedVolumes)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, override)
		})
	}
}

func TestRewritePorts(t *testing.T) {
	tests := []struct {
		name        string
		original    []PortMapping
		assignments []ports.Assignment
		want        []PortMapping
	}{
		{
			name: "multiple_ports",
			original: []PortMapping{
				{Target: 80, Published: 8080, HostIP: "127.0.0.1"},
				{Target: 443, Published: 8443, HostIP: "127.0.0.1"},
			},
			assignments: []ports.Assignment{
				{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1", HostPort: 41000},
				{Service: "web", ContainerPort: 443, HostIP: "127.0.0.1", HostPort: 41001},
			},
			want: []PortMapping{
				{Target: 80, Published: 41000, HostIP: "127.0.0.1", Protocol: "tcp"},
				{Target: 443, Published: 41001, HostIP: "127.0.0.1", Protocol: "tcp"},
			},
		},
		{
			name:        "protocol_defaults_to_tcp",
			original:    []PortMapping{{Target: 80, Published: 8080}},
			assignments: []ports.Assignment{{Service: "web", ContainerPort: 80, HostPort: 41000}},
			want:        []PortMapping{{Target: 80, Published: 41000, HostIP: "", Protocol: "tcp"}},
		},
		{
			name:        "hostip_empty_matches_127",
			original:    []PortMapping{{Target: 80, Published: 8080, HostIP: ""}},
			assignments: []ports.Assignment{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1", HostPort: 41000}},
			want:        []PortMapping{{Target: 80, Published: 41000, HostIP: "127.0.0.1", Protocol: "tcp"}},
		},
		{
			name: "unmatched_port_skipped",
			original: []PortMapping{
				{Target: 80, Published: 8080},
				{Target: 3306, Published: 33060},
			},
			assignments: []ports.Assignment{
				{Service: "web", ContainerPort: 80, HostPort: 41000},
			},
			want: []PortMapping{
				{Target: 80, Published: 41000, Protocol: "tcp"},
			},
		},
		{
			name:        "empty_original_returns_nil",
			original:    nil,
			assignments: []ports.Assignment{{Service: "web", ContainerPort: 80, HostPort: 41000}},
			want:        nil,
		},
		{
			name:        "empty_assignments_returns_nil",
			original:    []PortMapping{{Target: 80, Published: 8080}},
			assignments: nil,
			want:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewritePorts(tt.original, tt.assignments)
			if len(got) != len(tt.want) {
				t.Fatalf("length: got %d, want %d (got %#v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("port %d: got %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWriteOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".docktree", "generated", "x.yml")
	name := "instance-web"
	err := WriteOverride(&Override{Services: map[string]ServiceOverride{"web": {ContainerName: &name, Ports: PortOverride{{Target: 80, Published: 41000, HostIP: "127.0.0.1", Protocol: "tcp"}}}}}, path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "instance-web") || !strings.Contains(string(data), "published: 41000") {
		t.Fatalf("override not written: %s", data)
	}
}
