package compose

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Warning struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

var numericPortKey = regexp.MustCompile(`(^|_)(PORT|PUBLISHED)`)
var numericValue = regexp.MustCompile(`^[0-9]+$`)

func CheckEnvFile(dir string) ([]Warning, error) {
	file, err := os.Open(filepath.Join(dir, ".env"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var warnings []Warning
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, `"`) && !strings.HasPrefix(value, `'`) {
			if before, _, ok := strings.Cut(value, " #"); ok {
				value = strings.TrimSpace(before)
			}
		}
		value = strings.Trim(value, `"'`)
		switch {
		case key == "COMPOSE_PROJECT_NAME":
			warnings = append(warnings, Warning{Key: key, Value: value, Message: "COMPOSE_PROJECT_NAME is set; Docktree will pass -p so the generated project name wins."})
		case key == "COMPOSE_FILE":
			warnings = append(warnings, Warning{Key: key, Value: value, Message: "COMPOSE_FILE is set; Docktree will use it when selecting Compose files."})
		case numericPortKey.MatchString(key) && numericValue.MatchString(value):
			warnings = append(warnings, Warning{Key: key, Value: value, Message: "if you set a port in your env, you might need to update the app after docktree remaps your ports."})
		}
	}
	return warnings, scanner.Err()
}
