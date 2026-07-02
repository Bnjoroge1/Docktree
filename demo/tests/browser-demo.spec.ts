import { test, expect } from "@playwright/test";
import { execSync } from "child_process";
import * as path from "path";

const REPO = process.env.REPO || path.resolve(__dirname, "../..");
const DOCKTREE = process.env.DOCKTREE || path.join(REPO, "docktree");

function getPort(instanceFragment: string): number {
  const raw = execSync(`"${DOCKTREE}" ports --all --json`, {
    encoding: "utf-8",
    timeout: 10000,
  });
  const data = JSON.parse(raw);
  for (const entry of data.entries || []) {
    const webPort = (entry.ports || []).find(
      (p: { service: string; container_port: number }) =>
        p.service === "web" && p.container_port === 3000
    );
    if (webPort && entry.instance.includes(instanceFragment)) {
      return webPort.host_port;
    }
  }
  throw new Error(`Could not find port for instance matching "${instanceFragment}"`);
}

test("both app versions are live simultaneously with different content", async ({ page }) => {
  const mainPort = getPort("demo-main");
  const featurePort = getPort("demo-feature");

  // Open Main app.
  await page.goto(`http://localhost:${mainPort}`, { waitUntil: "networkidle" });
  await expect(page.locator("h1")).toHaveText("Docktree Demo App");
  await expect(page.locator(".badge")).toHaveText("Main checkout");
  await expect(page.locator(".version")).toHaveText("Version A: Main");
  await page.screenshot({ path: path.join(REPO, "demo", "out", "browser-main.png") });

  // Open Feature app.
  await page.goto(`http://localhost:${featurePort}`, { waitUntil: "networkidle" });
  await expect(page.locator("h1")).toHaveText("Docktree Demo App");
  await expect(page.locator(".badge")).toHaveText("Feature checkout");
  await expect(page.locator(".version")).toHaveText("Version B: Feature");
  await page.screenshot({ path: path.join(REPO, "demo", "out", "browser-feature.png") });

  // Side-by-side view.
  await page.setContent(`
    <html><body style="margin:0;display:flex;height:100vh;background:#0a0a0a;">
      <iframe src="http://localhost:${mainPort}" style="flex:1;border:none;"></iframe>
      <iframe src="http://localhost:${featurePort}" style="flex:1;border:none;"></iframe>
    </body></html>
  `);
  await page.waitForTimeout(3000);
  await page.screenshot({ path: path.join(REPO, "demo", "out", "browser-both.png") });
});
