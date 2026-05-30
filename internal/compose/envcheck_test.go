package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckEnvFileWarnings(t *testing.T) {
	dir := t.TempDir()
	data := []byte("COMPOSE_PROJECT_NAME=myapp\nCOMPOSE_FILE=compose.yml\nWEB_PORT=8080\nDATABASE_URL=postgres://x\nAPI_KEY=secret\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	warnings, err := CheckEnvFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 3 {
		t.Fatalf("warning count = %d, want 3: %#v", len(warnings), warnings)
	}
}

func TestCheckEnvFileMissing(t *testing.T) {
	warnings, err := CheckEnvFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if warnings != nil {
		t.Fatalf("warnings = %#v, want nil", warnings)
	}
}

func TestCheckEnvFileEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		wantWarn int
	}{
		{
			name:     "whitespace around key",
			env:      "  WEB_PORT  =8080\n",
			wantWarn: 1,
		},
		{
			name:     "whitespace around value",
			env:      "WEB_PORT=  8080  \n",
			wantWarn: 1,
		},
		{
			name:     "double-quoted value",
			env:      `WEB_PORT="8080"` + "\n",
			wantWarn: 1,
		},
		{
			name:     "single-quoted value",
			env:      `WEB_PORT='8080'` + "\n",
			wantWarn: 1,
		},
		{
			name:     "empty value no warning",
			env:      "WEB_PORT=\n",
			wantWarn: 0,
		},
		{
			name:     "PORT as prefix",
			env:      "PORT_8080=8080\n",
			wantWarn: 1,
		},
		{
			name:     "PORT prefix non-numeric value no warning",
			env:      "PORT_HTTP=http\n",
			wantWarn: 0,
		},
		{
			name:     "inline comment stripped",
			env:      "WEB_PORT=8080 # comment\n",
			wantWarn: 1,
		},
		{
			name:     "quoted hash preserved",
			env:      `WEB_PORT="8080#tag"` + "\n",
			wantWarn: 0,
		},
		{
			name:     "comment line skipped",
			env:      "# WEB_PORT=8080\n",
			wantWarn: 0,
		},
		{
			name:     "empty line skipped",
			env:      "\n\nWEB_PORT=8080\n",
			wantWarn: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tt.env), 0o644); err != nil {
				t.Fatal(err)
			}
			warnings, err := CheckEnvFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != tt.wantWarn {
				t.Errorf("warning count = %d, want %d; got %#v", len(warnings), tt.wantWarn, warnings)
			}
		})
	}
}
