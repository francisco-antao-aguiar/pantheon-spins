import { expect, type Page } from "@playwright/test";

/** Registers a fresh player (10,000 Coins) and lands in the lobby. */
export async function register(page: Page, prefix = "e2e") {
  const id = Date.now().toString(36) + Math.random().toString(36).slice(2, 5);
  const user = { username: `${prefix}_${id}`.slice(0, 20), email: `${prefix}_${id}@example.test`, password: `${prefix}-password-${id}` };
  await page.goto("/register");
  await page.getByLabel("Username").fill(user.username);
  await page.getByLabel("Email").fill(user.email);
  await page.getByLabel("Password").fill(user.password);
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByRole("heading", { name: "Choose your realm" })).toBeVisible();
  return user;
}

/** Reads the balance from the game's top bar (locale-formatted digits). */
export async function balance(page: Page) {
  const text = await page.locator("header").innerText();
  return Number(text.replace(/[^\d]/g, ""));
}

/** Opens a game from the lobby and waits for the spin button. */
export async function openGame(page: Page, name: RegExp, id: string) {
  await page.getByRole("link", { name }).click();
  await expect(page).toHaveURL(new RegExp(`/play/${id}$`));
  await expect(page.locator("canvas")).toBeVisible({ timeout: 45_000 });
  const spin = page.getByRole("button", { name: "Spin", exact: true });
  await expect(spin).toBeEnabled({ timeout: 30_000 });
  return spin;
}
