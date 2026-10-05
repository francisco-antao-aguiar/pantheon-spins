/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    // File events don't cross Docker bind mounts on Windows/macOS; poll there.
    watch: process.env.VITE_POLL ? { usePolling: true, interval: 300 } : undefined,
    proxy: {
      // Same-origin API in dev: cookies and CSRF work exactly as in production.
      "/api": { target: process.env.VITE_API_PROXY ?? "http://localhost:8080" },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
  },
});
