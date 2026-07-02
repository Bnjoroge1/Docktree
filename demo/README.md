# Docktree Demo

A reproducible demo that shows Docktree solving the **"two versions of the same app running at once"** problem.

## What the demo proves

Without Docktree, two git worktrees of the same Docker Compose app would fight over:
- **Host ports** — both want `3000:3000`
- **Container names** — both want `docktree-demo-web`

With Docktree, each worktree gets its own isolated Compose project with unique allocated ports and rewritten container names — so both versions run simultaneously.

The demo tells a three-act product story:

1. **The conflict** — show the shared compose file and explain why two worktrees collide.
2. **The isolation** — `docktree up` in each worktree, each gets a unique port.
3. **The payoff** — browser shows both app versions live at the same time, with visibly different UI.

## Prerequisites

| Tool | Required for | Install |
|------|-------------|---------|
| Go 1.25+ | Building Docktree | [go.dev](https://go.dev/dl/) |
| Docker + Docker Compose | Running the sample app | [docker.com](https://docker.com) |
| `just` | Build convenience commands | `brew install just` |
| python3 | Port extraction in scripts | (preinstalled on macOS/Linux) |
| [VHS](https://github.com/charmbracelet/vhs) | Terminal GIF/MP4 recordings (optional) | `brew install vhs` |
| Node.js 18+ | Browser recordings (optional) | [nodejs.org](https://nodejs.org) |
| Playwright | Browser video/screenshots (optional) | `npx playwright install chromium` |
| ffmpeg | Final video assembly (optional) | `brew install ffmpeg` |

## Quick start

From the repository root:

```bash
# 1. Build Docktree
just build

# 2. Create two demo worktrees with different app versions
demo/scripts/setup-demo.sh

# 3. Start both versions with Docktree
demo/scripts/run-demo.sh

# 4. Verify both are live simultaneously
demo/scripts/show-both.sh
```

Open the URLs printed by `show-both.sh` in your browser to see two different versions of the app running at the same time.

## Full demo flow (with recordings)

### Step 1 — Build Docktree

```bash
just build
```

This creates a `docktree` binary in the repo root.

### Step 2 — Prepare demo worktrees

```bash
demo/scripts/setup-demo.sh
```

Creates a self-contained demo git repo at `demo/demo-repo/` (separate from the Docktree repo, so docktree doesn't inherit shared-services config). Then creates two git worktrees from it:
- `demo/worktrees/main` — branch `demo-main`, blue accent, "Main checkout"
- `demo/worktrees/feature` — branch `demo-feature`, emerald accent, "Feature checkout"

Each worktree has a different `version.json` that changes the app's appearance.

### Step 3 — Start both versions with Docktree

```bash
demo/scripts/run-demo.sh
```

Runs `docktree up` in each worktree. Both use the same compose file (`demo/sample-app/docker-compose.yml`) with the same service name and port `3000:3000`. Docktree generates isolated overrides with unique ports.

### Step 4 — Open browser URLs

```bash
demo/scripts/show-both.sh
```

Prints both allocated URLs and curls each app's `/api` endpoint, showing different version text.

### Step 5 — Record terminal clips

```bash
vhs demo/recordings/01-problem.tape
vhs demo/recordings/02-docktree-up-main.tape
vhs demo/recordings/03-docktree-up-feature.tape
vhs demo/recordings/04-two-worktrees.tape
vhs demo/recordings/05-docker-proxy.tape
vhs demo/recordings/06-docker-tunnel.tape
```

Output goes to `demo/out/`. The tapes tell the story:
- `01-problem` — explains the port/container name conflict
- `02-docktree-up-main` — starts Worktree A, shows allocated port
- `03-docktree-up-feature` — starts Worktree B, shows a different port
- `04-two-worktrees` — shows both running, curls both APIs
- `05-docker-proxy` — reverse proxy routing by hostname to worktree ports
- `06-docker-tunnel` — exopses a worktree externally via Cloudflare Tunnel

### Step 6 — Record browser clips

```bash
cd demo
npm install
npx playwright install chromium
npm run record
```

This launches a headless Chromium, navigates to both app versions, and records:
- `demo/out/browser-main.png` — Main app screenshot
- `demo/out/browser-feature.png` — Feature app screenshot
- `demo/out/browser-both.png` — side-by-side screenshot
- `demo/out/browser-demo.webm` — video of the full sequence

Alternatively, run the Playwright test (which also asserts the version text):

```bash
cd demo
npm install
npx playwright install chromium
npm test
```

### Step 7 — Render final video

```bash
demo/scripts/render-demo.sh
```

If `vhs` and `ffmpeg` are available, this:
1. Renders all `.tape` files to MP4
2. Concatenates terminal clips into `demo/out/terminal-demo.mp4`
3. Converts the browser recording to `demo/out/browser-demo.mp4`
4. Creates a side-by-side composition: `demo/out/docktree-demo.mp4`

If tools are missing, the script prints install instructions and exits gracefully.

## Cleanup

```bash
demo/scripts/teardown-demo.sh
```

Stops both docktree instances, cleans orphaned resources, and removes the demo worktrees and the demo git repo.

## File structure

```
demo/
├── README.md                      ← this file
├── .gitignore
├── package.json                   ← Playwright + tsx deps
├── playwright.config.ts           ← Playwright test config
├── sample-app/                    ← the demo web app (source)
│   ├── main.go                    ← Go HTTP server (stdlib only)
│   ├── go.mod
│   ├── version.json               ← version config (differs per branch)
│   ├── Dockerfile                 ← multi-stage Go build
│   └── docker-compose.yml         ← shared compose (service: web, port: 3000)
├── scripts/
│   ├── setup-demo.sh              ← create demo repo + worktrees
│   ├── run-demo.sh                ← docktree up in both worktrees
│   ├── show-both.sh               ← prove both live simultaneously
│   ├── teardown-demo.sh           ← stop + clean + remove worktrees/repo
│   ├── render-demo.sh             ← ffmpeg assembly
│   └── record-browser-demo.ts     ← Playwright browser recording
├── recordings/                    ← VHS tape files
│   ├── 01-problem.tape
│   ├── 02-docktree-up-main.tape
│   ├── 03-docktree-up-feature.tape
│   ├── 04-two-worktrees.tape
│   ├── 05-docker-proxy.tape
│   ├── 06-docker-tunnel.tape
│   └── docktree-demo.tape
├── tests/
│   └── browser-demo.spec.ts       ← Playwright test (alternative to record script)
├── demo-repo/                     ← self-contained git repo (gitignored, created by setup)
├── worktrees/                     ← git worktrees (gitignored, created by setup)
└── out/                           ← generated recordings (gitignored)
```

## Troubleshooting

**"docktree binary not found"** — Run `just build` from the repo root first.

**"Could not find both ports"** — Make sure both instances are running: `./docktree status --all`. If not, run `demo/scripts/run-demo.sh`.

**Port already in use** — Run `demo/scripts/teardown-demo.sh` to clean up, then start fresh.

**Docker build fails** — Ensure Docker daemon is running (`docker info`). The sample app uses `golang:1.25-alpine` which will be pulled on first build.

**VHS tapes don't render** — VHS requires a terminal emulator. Install with `brew install vhs` (macOS).

**Playwright fails** — Run `npx playwright install chromium` after `npm install`.
