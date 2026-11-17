import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const backend = process.env.ANKER_BACKEND || "http://127.0.0.1:8087";
// The backend rejects writes whose Origin differs from its own Host, so the
// dev and preview servers present themselves as the backend origin.
const proxy = {
  "/api": { target: backend, changeOrigin: true, headers: { origin: backend } },
};

export default defineConfig({
  plugins: [react()],
  build: { outDir: "../internal/webassets/dist", emptyOutDir: true },
  server: { proxy },
  preview: { proxy },
});
