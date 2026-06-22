package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusComposeFilesParse(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "corpus")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			status := corpusStatus(t, filepath.Join(dir, "SOURCE"))
			_, err := ParseFile(filepath.Join(dir, "compose.yml"))
			if strings.HasPrefix(status, "known-gap:") {
				if err == nil {
					t.Fatalf("%s now parses; update SOURCE status from %q to parses", name, status)
				}
				return
			}
			if status != "parses" {
				t.Fatalf("unsupported SOURCE status %q", status)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func corpusStatus(t *testing.T, sourcePath string) string {
	t.Helper()
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if status, ok := strings.CutPrefix(line, "status: "); ok {
			return status
		}
	}
	t.Fatalf("%s missing status", sourcePath)
	return ""
}
