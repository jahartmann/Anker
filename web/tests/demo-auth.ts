import { test, type APIRequestContext } from "@playwright/test";
import { readFile, writeFile } from "node:fs/promises";

// One login per fresh demo server keeps large suites below the real login limit.
// Reuse only test cookies, in a private temporary file outside the repository.
export async function demoCookies(request: APIRequestContext) {
  const run = process.env.ANKER_BROWSER_RUN_ID;
  // Cookies are bound to a server/catalog, even when both servers use 127.0.0.1.
  const port = new URL(
    test.info().project.use.baseURL || "http://127.0.0.1:8088",
  ).port;
  const path =
    run && /^\d+$/.test(run)
      ? "/tmp/anker-test-cookies-" + run + "-" + port + ".json"
      : undefined;
  if (path) {
    try {
      return JSON.parse(await readFile(path, "utf8")) as Awaited<
        ReturnType<APIRequestContext["storageState"]>
      >["cookies"];
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    }
  }
  const response = await request.post("/api/login", {
    headers: { "X-Anker-Request": "1" },
    data: { name: "demo", password: "anker-demo-2026" },
  });
  if (!response.ok())
    throw new Error(
      "Demo-Testanmeldung fehlgeschlagen: HTTP " + response.status(),
    );
  const cookies = (await request.storageState()).cookies;
  if (path)
    await writeFile(path, JSON.stringify(cookies), { mode: 0o600, flag: "wx" });
  return cookies;
}
