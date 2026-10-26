import { defineConfig } from "@playwright/test";
const runID = (process.env.ANKER_BROWSER_RUN_ID ||= String(Date.now()));
const productionRoot = "/tmp/anker-production-browser-" + runID;
export default defineConfig({
  testDir: "tests",
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:8088",
    headless: true,
    viewport: { width: 1440, height: 1000 },
  },
  projects: [
    {
      name: "demo",
      testMatch: "usability-*.spec.ts",
      testIgnore: "usability-draft-comparison.spec.ts",
    },
    {
      // These workflows change the real demo filesystem and create new backups.
      // Keep that state away from history/evidence fixtures in the demo project.
      name: "integration",
      testMatch: "app.spec.ts",
      use: { baseURL: "http://127.0.0.1:8091" },
    },
    {
      name: "draft",
      testMatch: "usability-draft-comparison.spec.ts",
      use: { baseURL: "http://127.0.0.1:8090" },
    },
    {
      name: "production",
      testMatch: "production.spec.ts",
      use: { baseURL: "http://127.0.0.1:8089" },
    },
  ],
  webServer: [
    {
      command: `../bin/anker --data /tmp/anker-integration-browser-${runID} --listen 127.0.0.1:8091 demo`,
      url: "http://127.0.0.1:8091",
      reuseExistingServer: false,
    },
    {
      command: `../bin/anker --data /tmp/anker-draft-browser-${runID} --listen 127.0.0.1:8090 demo`,
      url: "http://127.0.0.1:8090",
      reuseExistingServer: false,
    },
    {
      command:
        "../bin/anker --data /tmp/anker-browser-" +
        runID +
        " --listen 127.0.0.1:8088 demo",
      url: "http://127.0.0.1:8088",
      reuseExistingServer: false,
    },
    {
      command: `../bin/anker --data ${productionRoot} init && ../bin/anker --data ${productionRoot} --listen 127.0.0.1:8089 serve`,
      env: { ANKER_INITIAL_PASSWORD: "production-browser-test-password" },
      url: "http://127.0.0.1:8089",
      reuseExistingServer: false,
    },
  ],
  timeout: 30000,
  reporter: "list",
});
