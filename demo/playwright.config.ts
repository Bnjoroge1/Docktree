import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  outputDir: "../out/test-results",
  timeout: 30000,
  use: {
    headless: true,
    viewport: { width: 1200, height: 500 },
    video: "on",
    screenshot: "on",
  },
  reporter: [["list"]],
});
