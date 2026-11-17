import { defineConfig } from "@playwright/test";

// Runs the demo UI tests against a production build of the current sources,
// served by Vite, so frontend changes can be tested without rebuilding the Go
// binary that embeds the web assets. The binary only provides the API.
const runID = (process.env.ANKER_BROWSER_RUN_ID ||= String(Date.now()));
const backend = "http://127.0.0.1:8098";
const frontend = "http://127.0.0.1:8088";
const build = `/tmp/anker-web-build-${runID}`;

export default defineConfig({
  testDir: "tests",
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: frontend,
    headless: true,
    viewport: { width: 1440, height: 1000 },
    // Optional: an already installed Chromium, when the pinned download is missing.
    launchOptions: { executablePath: process.env.ANKER_CHROMIUM || undefined },
  },
  projects: [
    {
      name: "demo",
      testMatch: "usability-*.spec.ts",
      testIgnore: "usability-draft-comparison.spec.ts",
    },
  ],
  webServer: [
    {
      command: `../bin/anker --data /tmp/anker-browser-${runID} --listen 127.0.0.1:8098 demo`,
      url: backend,
      reuseExistingServer: false,
    },
    {
      command: `npx vite build --outDir ${build} --emptyOutDir && npx vite preview --outDir ${build} --host 127.0.0.1 --port 8088 --strictPort`,
      env: { ANKER_BACKEND: backend },
      url: frontend,
      reuseExistingServer: false,
    },
  ],
  timeout: 30000,
  reporter: "list",
});
