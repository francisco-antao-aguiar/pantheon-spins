import { defineConfig, devices } from "@playwright/test";

/**
 * End-to-end tests run against the Docker Compose stack, whose server is a
 * devtools build (needed to force a bonus). Start it first:
 *   docker compose up -d --build
 * Override the target with E2E_BASE_URL.
 */
export default defineConfig({
  testDir: "./e2e",
  timeout: 180_000,
  expect: { timeout: 20_000 },
  fullyParallel: false,
  // One at a time: each test renders WebGL in software, and two at once
  // starve each other (and the dev stack) on a laptop.
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
    trace: "retain-on-failure",
    // Shorter animations; the game still plays every step.
    reducedMotion: "reduce",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "mobile-portrait", use: { ...devices["Pixel 7"] } },
  ],
});
