/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      // Same-origin API in dev: cookies and CSRF work exactly as in production.
      "/api": { target: process.env.VITE_API_PROXY ?? "http://localhost:8080" },
    },
  },
  test: {
    environment: "jsdom",
  },
});
