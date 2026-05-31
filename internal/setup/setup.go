package setup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bnjoroge/docktree/internal/config"
)

type Options struct {
	SourceDir string
	TargetDir string
	Config    *config.Config
	Stdout    io.Writer
	Stderr    io.Writer
}

func Prepare(opts Options) error {
	if opts.Config == nil {
		return fmt.Errorf("setup config is nil")
	}
	if opts.SourceDir == "" {
		return fmt.Errorf("source dir is required")
	}
	if opts.TargetDir == "" {
		return fmt.Errorf("target dir is required")
	}
	if samePath(opts.SourceDir, opts.TargetDir) {
		return runCommands(opts.TargetDir, opts.Config.Setup.Run, opts.Stdout, opts.Stderr)
	}
	for _, rel := range opts.Config.Setup.Copy {
		if err := copyPath(opts.SourceDir, opts.TargetDir, rel); err != nil {
			return err
		}
	}
	for _, rel := range opts.Config.Setup.Symlink {
		if err := symlinkPath(opts.SourceDir, opts.TargetDir, rel); err != nil {
			return err
		}
	}
	return runCommands(opts.TargetDir, opts.Config.Setup.Run, opts.Stdout, opts.Stderr)
}

func runCommands(dir string, commands []string, stdout, stderr io.Writer) error {
	for _, command := range commands {
		if err := runCommand(dir, command, stdout, stderr); err != nil {
			return err
		}
	}
	return nil
}

func copyPath(sourceDir, targetDir, rel string) error {
	source := filepath.Join(sourceDir, rel)
	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("copy %s: %w", rel, err)
	}
	target := filepath.Join(targetDir, rel)
	if info.IsDir() {
		return copyDir(source, target)
	}
	return copyFile(source, target)
}

func copyDir(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := target
		if rel != "." {
			dest = filepath.Join(target, rel)
		}
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}
		return copyFileWithMode(path, dest, info.Mode())
	})
}

func copyFile(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	return copyFileWithMode(source, target, info.Mode())
}

func copyFileWithMode(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(target, mode)
}

func symlinkPath(sourceDir, targetDir, rel string) error {
	source := filepath.Join(sourceDir, rel)
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("symlink %s: %w", rel, err)
	}
	target := filepath.Join(targetDir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if existing, err := os.Lstat(target); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(target); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("symlink %s: target already exists", rel)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(source, target)
}

func runCommand(dir, command string, stdout, stderr io.Writer) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %q: %w", command, err)
	}
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.EvalSymlinks(a)
	bb, errB := filepath.EvalSymlinks(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
