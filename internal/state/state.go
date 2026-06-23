package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func lockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func unlockFile(f *os.File) error {
	if f == nil {
		return nil
	}
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Instance records the local metadata Docktree needs to manage one worktree.
type Instance struct {
	Name            string    `json:"name"`
	ProjectName     string    `json:"project_name"`
	RepoRoot        string    `json:"repo_root"`
	WorktreeRoot    string    `json:"worktree_root"`
	StateDirectory  string    `json:"state_directory,omitempty"`
	Branch          string    `json:"branch"`
	CreatedAt       time.Time `json:"created_at"`
	LastActiveAt    time.Time `json:"last_active_at"`
	ComposeFileHash string    `json:"compose_file_hash"`
	// ComposeFiles records the base compose files this instance was started
	// with (absolute paths). It is the source of truth for down/status so they
	// don't re-derive files from docktree.yml, which may differ from a `-f` run.
	ComposeFiles []string `json:"compose_files,omitempty"`
}

func GlobalConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "docktree")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".docktree-config"
	}
	return filepath.Join(home, ".config", "docktree")
}

func LoadGlobalInstances(configDir string) (map[string]Instance, error) {
	if configDir == "" {
		configDir = GlobalConfigDir()
	}
	data, err := os.ReadFile(filepath.Join(configDir, "instances.json"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Instance{}, nil
	}
	if err != nil {
		return nil, err
	}
	var instances map[string]Instance
	if err := json.Unmarshal(data, &instances); err != nil {
		return nil, err
	}
	if instances == nil {
		instances = map[string]Instance{}
	}
	return instances, nil
}

func LoadGlobalState(configDir string) (map[string]Instance, error) {
	return LoadGlobalInstances(configDir)
}

func atomicWriteFile(path string, data []byte) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0o644); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	removeTmp = false
	return nil
}

func SaveGlobalInstances(configDir string, instances map[string]Instance) error {
	if configDir == "" {
		configDir = GlobalConfigDir()
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(configDir, "instances.json"), append(data, '\n'))
}

func UpsertGlobalInstance(configDir string, inst *Instance) error {
	if inst == nil || inst.Name == "" {
		return nil
	}
	if configDir == "" {
		configDir = GlobalConfigDir()
	}
	lf, err := lockFile(filepath.Join(configDir, "instances.lock"))
	if err != nil {
		return err
	}
	defer func() { _ = unlockFile(lf) }()
	instances, err := LoadGlobalInstances(configDir)
	if err != nil {
		return err
	}
	instances[inst.Name] = *inst
	return SaveGlobalInstances(configDir, instances)
}

func RemoveGlobalInstance(configDir, name string) error {
	if name == "" {
		return nil
	}
	if configDir == "" {
		configDir = GlobalConfigDir()
	}
	lf, err := lockFile(filepath.Join(configDir, "instances.lock"))
	if err != nil {
		return err
	}
	defer func() { _ = unlockFile(lf) }()
	instances, err := LoadGlobalInstances(configDir)
	if err != nil {
		return err
	}
	delete(instances, name)
	return SaveGlobalInstances(configDir, instances)
}

func RemoveStateDir(inst *Instance) error {
	if inst == nil {
		return nil
	}
	path := inst.StateDirectory
	if path == "" && inst.WorktreeRoot != "" {
		path = filepath.Join(inst.WorktreeRoot, ".docktree")
	}
	if path == "" {
		return nil
	}
	err := os.RemoveAll(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func EnsureStateDir(worktreeRoot, stateDir string) error {
	if stateDir == "" {
		stateDir = ".docktree"
	}
	root := statePath(worktreeRoot, stateDir)
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		return err
	}
	return nil
}

func LoadInstance(stateDir string) (*Instance, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	var inst Instance
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

func SaveInstance(stateDir string, inst *Instance) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(stateDir, "state.json"), append(data, '\n'))
}

func HashFiles(paths []string) (string, error) {
	hash := sha256.New()
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, path+"\n"); err != nil {
			file.Close()
			return "", err
		}
		if _, err := io.Copy(hash, file); err != nil {
			file.Close()
			return "", err
		}
		file.Close()
		if _, err := io.WriteString(hash, "\n"); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func StatePath(worktreeRoot, stateDir string) string {
	return statePath(worktreeRoot, stateDir)
}

func statePath(worktreeRoot, stateDir string) string {
	if stateDir == "" {
		stateDir = ".docktree"
	}
	if filepath.IsAbs(stateDir) {
		return stateDir
	}
	return filepath.Join(worktreeRoot, stateDir)
}
