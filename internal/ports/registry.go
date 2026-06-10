package ports

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Registry struct {
	Dir      string
	lockFile *os.File
}

func NewRegistry() *Registry {
	return &Registry{Dir: defaultDir()}
}

func (r *Registry) EnsureGlobalDir() error {
	return os.MkdirAll(r.dir(), 0o755)
}

func (r *Registry) Lock() error {
	if err := r.EnsureGlobalDir(); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(r.dir(), "ports.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return err
	}
	r.lockFile = file
	return nil
}

func (r *Registry) Unlock() error {
	if r.lockFile == nil {
		return nil
	}
	err := syscall.Flock(int(r.lockFile.Fd()), syscall.LOCK_UN)
	closeErr := r.lockFile.Close()
	r.lockFile = nil
	if err != nil {
		return err
	}
	return closeErr
}

func (r *Registry) Load() (map[string][]Assignment, error) {
	data, err := os.ReadFile(filepath.Join(r.dir(), "ports.json"))
	if os.IsNotExist(err) {
		return map[string][]Assignment{}, nil
	}
	if err != nil {
		return nil, err
	}
	var registry map[string][]Assignment
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, err
	}
	if registry == nil {
		registry = map[string][]Assignment{}
	}
	return registry, nil
}

func (r *Registry) Save(registry map[string][]Assignment) error {
	if err := r.EnsureGlobalDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir(), "ports.json"), append(data, '\n'), 0o644)
}

func (r *Registry) Allocate(instanceName string, needed []PortRequest, portRange Range) ([]Assignment, error) {
	registry, err := r.Load()
	if err != nil {
		return nil, err
	}
	existing := registry[instanceName]
	existing = pruneUnavailable(existing, needed)
	if covers(existing, needed) {
		registry[instanceName] = existing
		if err := r.Save(registry); err != nil {
			return nil, err
		}
		return filterExisting(existing, needed), nil
	}
	used := usedPorts(registry)
	assignments := append([]Assignment(nil), existing...)
	for _, request := range needed {
		if hasAssignment(assignments, request) {
			continue
		}
		hostIP := request.HostIP
		if hostIP == "" {
			hostIP = "127.0.0.1"
		}
		port, err := firstFree(hostIP, portRange, used)
		if err != nil {
			return nil, err
		}
		used[port] = true
		assignments = append(assignments, Assignment{
			Service:       request.Service,
			ContainerPort: request.ContainerPort,
			HostIP:        hostIP,
			HostPort:      port,
		})
	}
	registry[instanceName] = assignments
	if err := r.Save(registry); err != nil {
		return nil, err
	}
	return filterExisting(assignments, needed), nil
}

func (r *Registry) Release(instanceName string) error {
	registry, err := r.Load()
	if err != nil {
		return err
	}
	delete(registry, instanceName)
	return r.Save(registry)
}

func ParseRange(s string) (Range, error) {
	left, right, ok := strings.Cut(strings.TrimSpace(s), "-")
	if !ok {
		return Range{}, fmt.Errorf("range must be min-max")
	}
	min, err := strconv.Atoi(left)
	if err != nil {
		return Range{}, err
	}
	max, err := strconv.Atoi(right)
	if err != nil {
		return Range{}, err
	}
	if min <= 0 || max < min || max > 65535 {
		return Range{}, fmt.Errorf("invalid port range %d-%d", min, max)
	}
	return Range{Min: min, Max: max}, nil
}

func (r *Registry) dir() string {
	if r.Dir != "" {
		return r.Dir
	}
	return defaultDir()
}

func defaultDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "docktree")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".docktree-config"
	}
	return filepath.Join(home, ".config", "docktree")
}

func covers(assignments []Assignment, needed []PortRequest) bool {
	for _, request := range needed {
		if !hasAssignment(assignments, request) {
			return false
		}
	}
	return true
}

func hasAssignment(assignments []Assignment, request PortRequest) bool {
	for _, assignment := range assignments {
		if assignment.Service == request.Service && assignment.ContainerPort == request.ContainerPort && sameHost(assignment.HostIP, request.HostIP) {
			return true
		}
	}
	return false
}

func filterExisting(assignments []Assignment, needed []PortRequest) []Assignment {
	var filtered []Assignment
	for _, request := range needed {
		for _, assignment := range assignments {
			if assignment.Service == request.Service && assignment.ContainerPort == request.ContainerPort && sameHost(assignment.HostIP, request.HostIP) {
				filtered = append(filtered, assignment)
				break
			}
		}
	}
	return filtered
}

func pruneUnavailable(assignments []Assignment, needed []PortRequest) []Assignment {
	var kept []Assignment
	for _, assignment := range assignments {
		neededAssignment := false
		for _, request := range needed {
			if assignment.Service == request.Service && assignment.ContainerPort == request.ContainerPort && sameHost(assignment.HostIP, request.HostIP) {
				neededAssignment = true
				break
			}
		}
		if neededAssignment && !portAvailable(assignment.HostIP, assignment.HostPort) {
			continue
		}
		kept = append(kept, assignment)
	}
	return kept
}

func usedPorts(registry map[string][]Assignment) map[int]bool {
	used := map[int]bool{}
	for _, assignments := range registry {
		for _, assignment := range assignments {
			used[assignment.HostPort] = true
		}
	}
	return used
}

func firstFree(hostIP string, portRange Range, used map[int]bool) (int, error) {
	for port := portRange.Min; port <= portRange.Max; port++ {
		if used[port] {
			continue
		}
		if !portAvailable(hostIP, port) {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("no free ports in range %d-%d", portRange.Min, portRange.Max)
}

func portAvailable(hostIP string, port int) bool {
	if hostIP == "" {
		hostIP = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(hostIP, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func sameHost(a, b string) bool {
	return a == b || (a == "127.0.0.1" && b == "") || (a == "" && b == "127.0.0.1")
}
