package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type VersionConfig struct {
	VersionLabel  string `json:"version_label"`
	BranchLabel   string `json:"branch_label"`
	CheckoutLabel string `json:"checkout_label"`
	BgColor       string `json:"bg_color"`
	AccentColor   string `json:"accent_color"`
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Docktree Demo App</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: radial-gradient(circle at 50% 40%, {{BG_COLOR}} 0%, #050505 130%);
      color: #fff;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
    }
    .card {
      background: rgba(255, 255, 255, 0.04);
      border: 1px solid rgba(255, 255, 255, 0.08);
      border-radius: 20px;
      padding: 56px 64px;
      text-align: center;
      backdrop-filter: blur(12px);
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
    }
    .badge {
      display: inline-block;
      background: {{ACCENT_COLOR}};
      color: #000;
      padding: 6px 16px;
      border-radius: 24px;
      font-size: 13px;
      font-weight: 700;
      letter-spacing: 0.5px;
      margin-bottom: 20px;
      text-transform: uppercase;
    }
    h1 {
      font-size: 32px;
      font-weight: 700;
      margin-bottom: 6px;
      letter-spacing: -0.5px;
    }
    .version {
      font-size: 22px;
      font-weight: 600;
      color: {{ACCENT_COLOR}};
      margin-bottom: 32px;
    }
    .info-grid {
      display: grid;
      gap: 10px;
      text-align: left;
      font-size: 14px;
      min-width: 320px;
    }
    .info-row {
      display: flex;
      justify-content: space-between;
      gap: 32px;
      padding: 10px 16px;
      background: rgba(255, 255, 255, 0.03);
      border-radius: 8px;
    }
    .info-label { color: rgba(255, 255, 255, 0.5); }
    .info-value {
      color: #fff;
      font-weight: 500;
      font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="badge">{{CHECKOUT_LABEL}}</div>
    <h1>Docktree Demo App</h1>
    <div class="version">{{VERSION_LABEL}}</div>
    <div class="info-grid">
      <div class="info-row">
        <span class="info-label">Branch</span>
        <span class="info-value">{{BRANCH_LABEL}}</span>
      </div>
      <div class="info-row">
        <span class="info-label">Container</span>
        <span class="info-value">{{HOSTNAME}}</span>
      </div>
      <div class="info-row">
        <span class="info-label">Container Port</span>
        <span class="info-value">3000</span>
      </div>
    </div>
  </div>
</body>
</html>`

func loadConfig() (VersionConfig, error) {
	var cfg VersionConfig
	for _, p := range []string{"version.json", "/app/version.json"} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", p, err)
		}
		return cfg, nil
	}
	return cfg, fmt.Errorf("version.json not found")
}

func renderHTML(cfg VersionConfig, hostname string) string {
	r := strings.NewReplacer(
		"{{BG_COLOR}}", cfg.BgColor,
		"{{ACCENT_COLOR}}", cfg.AccentColor,
		"{{CHECKOUT_LABEL}}", cfg.CheckoutLabel,
		"{{VERSION_LABEL}}", cfg.VersionLabel,
		"{{BRANCH_LABEL}}", cfg.BranchLabel,
		"{{HOSTNAME}}", hostname,
	)
	return r.Replace(htmlTemplate)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	hostname, _ := os.Hostname()
	html := renderHTML(cfg, hostname)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := json.MarshalIndent(map[string]string{
			"app":      "Docktree Demo App",
			"version":  cfg.VersionLabel,
			"branch":   cfg.BranchLabel,
			"checkout": cfg.CheckoutLabel,
			"hostname": hostname,
		}, "", "  ")
		w.Write(data)
	})

	fmt.Println("Docktree Demo App listening on :3000")
	if err := http.ListenAndServe(":3000", mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
