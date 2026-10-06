import { expect, test, type Page } from "@playwright/test";

const GAME = "halls-of-valhalla";

/** Reads the balance from the game's top bar (locale-formatted digits). */
async function balance(page: Page) {
  const text = await page.locator("header").innerText();
  return Number(text.replace(/[^\d]/g, ""));
}

/** "7 of 10 spins left" → { left: 7, total: 10, played: 3 } */
async function spinCounts(page: Page) {
  const text = await page.getByText(/\d+ of \d+ spins left/).innerText();
  const [, left, total] = /(\d+) of (\d+)/.exec(text)!.map(Number) as [number, number, number];
  return { left, total, played: total - left };
}

test("register, log in, spin, and resume a bonus after a reload", async ({ page }) => {
  const id = Date.now().toString(36) + Math.random().toString(36).slice(2, 5);
  const email = `e2e_${id}@example.test`;
  const password = `e2e-password-${id}`;

  // Register: new players start with 10,000 Coins.
  await page.goto("/register");
  await page.getByLabel("Username").fill(`e2e_${id}`);
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByRole("heading", { name: "Choose your realm" })).toBeVisible();

  // Sign out, then log back in.
  await page.goto("/profile");
  await page.getByRole("button", { name: "Sign out" }).last().click();
  await expect(page).toHaveURL(/\/login$/);
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Choose your realm" })).toBeVisible();

  // Open the game from the lobby.
  await page.getByRole("link", { name: /Halls of Valhalla/ }).click();
  await expect(page).toHaveURL(new RegExp(`/play/${GAME}$`));
  await expect(page.locator("canvas")).toBeVisible();
  const spin = page.getByRole("button", { name: "Spin", exact: true });
  await expect(spin).toBeEnabled();
  await page.getByRole("button", { name: "TURBO" }).click();
  expect(await balance(page)).toBe(10_000);

  // One paid spin: the server records it and the balance reflects bet and win.
  await spin.click();
  await expect(spin).toBeDisabled();
  await expect(spin).toBeEnabled();
  const history = await (await page.request.get("/api/v1/spins")).json();
  expect(history.items).toHaveLength(1);
  const wallet = await (await page.request.get("/api/v1/wallet")).json();
  expect(wallet.balance).toBe(10_000 - history.items[0].bet + history.items[0].win);
  expect(await balance(page)).toBe(wallet.balance);

  // Force a bonus (devtools build). Free spins start on their own.
  await page.getByRole("button", { name: "DEV bonus" }).click();
  await expect(page.getByText(/spins left/)).toBeVisible({ timeout: 60_000 });
  await expect.poll(async () => (await spinCounts(page)).played, { timeout: 60_000 }).toBeGreaterThan(0);
  await page.getByRole("button", { name: "Pause" }).click();
  const resume = page.getByRole("button", { name: "Continue" });
  await expect(resume).toBeVisible({ timeout: 60_000 });

  const paused = await (await page.request.get(`/api/v1/games/${GAME}/bonus`)).json();
  expect(paused.status).toBe("active");
  expect(paused.data.spinsLeft).toBe((await spinCounts(page)).left);

  // A new paid spin is refused while the bonus is unresolved.
  await expect(spin).toBeDisabled();

  // Reload: the bonus resumes on its own, exactly where it stopped.
  const firstAction = page.waitForRequest((r) => r.url().endsWith(`/games/${GAME}/bonus/actions`));
  await page.reload();
  expect((await firstAction).postDataJSON()).toEqual({ action: "spin", step: paused.step });

  // It plays through to the end.
  await expect(page.getByText(/spins left/)).toBeHidden({ timeout: 150_000 });
  await expect(spin).toBeEnabled();
  expect((await page.request.get(`/api/v1/games/${GAME}/bonus`)).status()).toBe(204);
  const after = await (await page.request.get("/api/v1/wallet")).json();
  expect(await balance(page)).toBe(after.balance);
});
