import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "tests",
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:8088",
    headless: true,
    viewport: { width: 1440, height: 1000 },
  },
  webServer: {
    command:
      "../bin/anker --data /tmp/anker-browser-" +
      Date.now() +
      " --listen 127.0.0.1:8088 demo",
    url: "http://127.0.0.1:8088",
    reuseExistingServer: false,
  },
  timeout: 30000,
  reporter: "list",
});
