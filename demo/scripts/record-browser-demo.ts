/**
 * record-browser-demo.ts
 *
 * Standalone Playwright script that records a browser video showing both
 * app versions live simultaneously.
 *
 * Usage:
 *   cd demo
 *   npm install
 *   npx playwright install chromium
 *   npm run record
 *
 * Requires both docktree instances to be running (run-demo.sh).
 */
import { chromium } from "playwright";
import { execSync } from "child_process";
import * as fs from "fs";
import * as path from "path";

const REPO = process.env.REPO || path.resolve(__dirname, "../..");
const DOCKTREE = process.env.DOCKTREE || path.join(REPO, "docktree");
const OUT_DIR = path.join(REPO, "demo", "out");

interface PortAssignment {
  service: string;
  container_port: number;
  host_ip: string;
  host_port: number;
}

interface PortsEntry {
  instance: string;
  ports: PortAssignment[];
}

interface PortsResult {
  instance?: string;
  all?: boolean;
  entries?: PortsEntry[];
}

function getPorts(): { main: number; feature: number } {
  const raw = execSync(`"${DOCKTREE}" ports --all --json`, {
    encoding: "utf-8",
    timeout: 10000,
  });
  const data: PortsResult = JSON.parse(raw);
  const entries = data.entries || [];

  let mainPort = 0;
  let featurePort = 0;

  for (const entry of entries) {
    // Only consider instances running the demo app (service "web" on port 3000).
    const webPort = entry.ports.find(
      (p) => p.service === "web" && p.container_port === 3000
    );
    if (!webPort) continue;

    if (entry.instance.includes("main")) {
      mainPort = webPort.host_port;
    }
    if (entry.instance.includes("feature")) {
      featurePort = webPort.host_port;
    }
  }

  if (!mainPort || !featurePort) {
    throw new Error(
      `Could not find both ports. Main: ${mainPort}, Feature: ${featurePort}.\n` +
        "Make sure both instances are running (demo/scripts/run-demo.sh)."
    );
  }

  return { main: mainPort, feature: featurePort };
}

async function main() {
  fs.mkdirSync(OUT_DIR, { recursive: true });

  console.log("▸ Reading docktree ports…");
  const { main: mainPort, feature: featurePort } = getPorts();
  const mainUrl = `http://localhost:${mainPort}`;
  const featureUrl = `http://localhost:${featurePort}`;
  console.log(`  Main:    ${mainUrl}`);
  console.log(`  Feature: ${featureUrl}`);

  console.log("▸ Launching browser…");
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1200, height: 500 },
    recordVideo: { dir: OUT_DIR, size: { width: 1200, height: 500 } },
  });
  const page = await context.newPage();

  // Step 1: Open Main app.
  console.log("▸ Recording Main app…");
  await page.goto(mainUrl, { waitUntil: "networkidle" });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: path.join(OUT_DIR, "browser-main.png") });

  // Step 2: Open Feature app.
  console.log("▸ Recording Feature app…");
  await page.goto(featureUrl, { waitUntil: "networkidle" });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: path.join(OUT_DIR, "browser-feature.png") });

  // Step 3: Show both side-by-side via iframes.
  console.log("▸ Recording side-by-side view…");
  await page.setContent(`
    <html>
      <head>
        <style>
          * { margin: 0; padding: 0; box-sizing: border-box; }
          body { background: #0a0a0a; display: flex; flex-direction: column; height: 100vh; font-family: sans-serif; }
          .header {
            background: #111;
            color: #fff;
            padding: 10px 20px;
            font-size: 14px;
            border-bottom: 1px solid #333;
            display: flex;
            gap: 24px;
          }
          .header span { color: #888; }
          .header b { color: #3b82f6; }
          .header .feat b { color: #10b981; }
          .split { display: flex; flex: 1; overflow: hidden; }
          .pane { flex: 1; border-right: 1px solid #333; overflow: hidden; position: relative; }
          .pane:last-child { border-right: none; }
          .pane-label {
            position: absolute;
            top: 8px;
            left: 8px;
            background: rgba(0,0,0,0.7);
            color: #fff;
            padding: 4px 10px;
            border-radius: 4px;
            font-size: 12px;
            z-index: 10;
          }
          iframe { width: 100%; height: 100%; border: none; }
        </style>
      </head>
      <body>
        <div class="header">
          <span>Docktree Demo — Two worktrees, same app, running simultaneously</span>
          <span>Main: <b>localhost:${mainPort}</b></span>
          <span class="feat">Feature: <b>localhost:${featurePort}</b></span>
        </div>
        <div class="split">
          <div class="pane">
            <div class="pane-label">Worktree A — demo-main</div>
            <iframe src="${mainUrl}"></iframe>
          </div>
          <div class="pane">
            <div class="pane-label">Worktree B — demo-feature</div>
            <iframe src="${featureUrl}"></iframe>
          </div>
        </div>
      </body>
    </html>
  `);
  await page.waitForTimeout(1000);
  // Wait for iframes to load.
  await page.waitForTimeout(3000);
  await page.screenshot({ path: path.join(OUT_DIR, "browser-both.png") });

  // Hold the side-by-side view for the video.
  await page.waitForTimeout(3000);

  console.log("▸ Closing browser (finalizing video)…");
  await context.close();
  await browser.close();

  // Rename the video file (Playwright generates a random name).
  const webmFiles = fs
    .readdirSync(OUT_DIR)
    .filter((f) => f.endsWith(".webm"));
  if (webmFiles.length > 0) {
    const oldPath = path.join(OUT_DIR, webmFiles[0]);
    const newPath = path.join(OUT_DIR, "browser-demo.webm");
    if (fs.existsSync(newPath)) fs.unlinkSync(newPath);
    fs.renameSync(oldPath, newPath);
    console.log(`  → browser-demo.webm`);
  }

  console.log("  → browser-main.png");
  console.log("  → browser-feature.png");
  console.log("  → browser-both.png");
  console.log("✓ Browser recording complete.");
}

main().catch((err) => {
  console.error("✗", err.message);
  process.exit(1);
});
