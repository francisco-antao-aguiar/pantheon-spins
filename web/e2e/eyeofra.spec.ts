import { expect, test, type Page } from "@playwright/test";
import { balance, openGame, register } from "./helpers";

const GAME = "eye-of-ra";

interface Tomb {
  step: number;
  status: string;
  data: { chamber: number; urns: { kind: string }[][] };
}

async function tomb(page: Page): Promise<Tomb | null> {
  const res = await page.request.get(`/api/v1/games/${GAME}/bonus`);
  return res.status() === 200 ? res.json() : null;
}

/** Opens sealed urns with the number keys until the bonus moves past `step`. */
async function pickUntil(page: Page, done: (t: Tomb | null) => boolean) {
  await expect
    .poll(
      async () => {
        for (const key of ["1", "2", "3", "4", "5", "6", "7", "8", "9"]) await page.keyboard.press(key);
        return done(await tomb(page));
      },
      { timeout: 120_000, intervals: [400] },
    )
    .toBe(true);
}

test("cascades, then a Tomb Explorer bonus that survives a reload", async ({ page }) => {
  await register(page, "ra");
  const spin = await openGame(page, /Eye of Ra/, GAME);
  await page.getByRole("button", { name: "TURBO" }).click();

  // A paid spin: the server's steps add up to its total and the wallet agrees.
  const spinRes = page.waitForResponse((r) => r.url().endsWith(`/games/${GAME}/spin`));
  await spin.click();
  const result = await (await spinRes).json();
  const stepSum = result.outcome.steps.reduce((a: number, s: { stepWin: number }) => a + s.stepWin, 0);
  expect(stepSum).toBe(result.totalWin);
  await expect(spin).toBeEnabled();
  expect(await balance(page)).toBe(result.balance);

  // Force Tomb Explorer and open the first urn.
  await page.getByRole("button", { name: "DEV bonus" }).click();
  await expect(page.getByText(/Tomb Explorer/)).toBeVisible({ timeout: 60_000 });
  await pickUntil(page, (t) => t === null || t.step >= 1);

  const before = await tomb(page);
  test.skip(before === null, "the first urn ended the bonus; nothing left to resume");
  const revealed = before!.data.urns.flat().filter((u) => u.kind !== "hidden").length;
  expect(revealed).toBeGreaterThanOrEqual(1);

  // Reload: the tomb comes back with the same urns open, and play continues.
  await page.reload();
  await expect(page.getByText(/Tomb Explorer/)).toBeVisible();
  const after = await tomb(page);
  expect(after!.step).toBe(before!.step);
  expect(after!.data.urns.flat().filter((u) => u.kind !== "hidden").length).toBe(revealed);
  await expect(spin).toBeDisabled();

  await pickUntil(page, (t) => t === null);
  await expect(page.getByText(/Tomb Explorer/)).toBeHidden({ timeout: 30_000 });
  await expect(spin).toBeEnabled();
  const wallet = await (await page.request.get("/api/v1/wallet")).json();
  expect(await balance(page)).toBe(wallet.balance);
});
