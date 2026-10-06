import { beforeEach, describe, expect, it, vi } from "vitest";
import { createApiClient, type BonusState, type GameInfo, type SpinResult } from "../api/client";
import { createSlotSession } from "./session";
import type { BonusStepResult, GameRenderer } from "./types";

const info: GameInfo = {
  id: "halls-of-valhalla",
  name: "Halls of Valhalla",
  theme: "norse",
  layout: "ways",
  reels: 5,
  rows: 4,
  volatility: "high",
  betLevels: [20, 40, 100, 200],
  defaultBet: 100,
  tags: [],
};

const grid = [["fehu"], ["uruz"], ["axe"], ["horn"], ["wild"]];

function spinResult(over: Partial<SpinResult> = {}): SpinResult {
  return {
    spinId: crypto.randomUUID(),
    gameId: info.id,
    bet: 100,
    outcome: { steps: [{ grid, wins: [], stepWin: 0 }], totalWin: 0 },
    totalWin: 0,
    winTier: "none",
    balance: 9_900,
    ...over,
  };
}

function bonusState(over: Partial<BonusState> = {}): BonusState {
  return {
    bonusId: "b1",
    gameId: info.id,
    kind: "free_spins",
    status: "active",
    bet: 100,
    totalWin: 0,
    step: 0,
    actions: [{ action: "spin" }],
    data: { god: "thor", spinsLeft: 2, spinsPlayed: 0, totalSpins: 2 },
    ...over,
  };
}

function stepResult(state: BonusState, stepWin: number, balance: number): BonusStepResult {
  return {
    state,
    stepWin,
    winTier: stepWin >= 1000 ? "big" : stepWin > 0 ? "normal" : "none",
    balance,
    outcome: { steps: [{ grid, wins: [], stepWin }], totalWin: stepWin },
  };
}

function fakeRenderer() {
  const calls: string[] = [];
  const r: GameRenderer = {
    init: async () => {},
    resize: () => {},
    startSpin: () => void calls.push("startSpin"),
    cancelSpin: async () => void calls.push("cancelSpin"),
    playSpin: async () => void calls.push("playSpin"),
    playBonusIntro: async () => void calls.push("bonusIntro"),
    playBonusStep: async () => void calls.push("bonusStep"),
    resumeBonus: () => void calls.push("resumeBonus"),
    playBonusEnd: async () => void calls.push("bonusEnd"),
    celebrate: async (_w, _b, tier) => void calls.push("celebrate:" + tier),
    skip: () => {},
    destroy: () => {},
  };
  return { r, calls };
}

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

/** A scripted server: each call to a path pops the next response. */
function setup(script: Record<string, Response[]>, balance = 10_000) {
  const requests: { path: string; body: unknown }[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(new URL(String(input), "http://app.test"), init);
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    if (path === "/auth/csrf") return json(200, { token: "t" });
    requests.push({ path: `${req.method} ${path}`, body: req.method === "POST" ? await req.clone().json() : undefined });
    const next = script[`${req.method} ${path}`]?.shift();
    return next ?? json(500, { code: "internal", message: `unscripted ${req.method} ${path}` });
  });
  const api = createApiClient({ baseUrl: "http://app.test/api/v1", fetch: fetchMock as typeof fetch });
  const { r, calls } = fakeRenderer();
  let bal = balance;
  const store = createSlotSession(info, 100, {
    api,
    renderer: r,
    getBalance: () => bal,
    setBalance: (b) => (bal = b),
    options: () => ({ turbo: true }),
    delay: async () => {},
  });
  return { store, calls, requests, balance: () => bal };
}

const SPIN = "POST /games/halls-of-valhalla/spin";
const ACTION = "POST /games/halls-of-valhalla/bonus/actions";
const BONUS = "GET /games/halls-of-valhalla/bonus";

describe("slot session", () => {
  beforeEach(() => {
    document.cookie = "ps_csrf=; max-age=0";
  });

  it("plays a spin and applies the server's balance and win", async () => {
    const { store, calls, requests, balance } = setup({ [SPIN]: [json(200, spinResult({ totalWin: 40, winTier: "normal", balance: 9_940 }))] });
    await store.getState().spin();
    expect(requests[0]).toEqual({ path: SPIN, body: { bet: 100 } });
    expect(calls).toEqual(["startSpin", "playSpin"]);
    expect(balance()).toBe(9_940);
    expect(store.getState()).toMatchObject({ phase: "idle", lastWin: 40 });
  });

  it("celebrates big wins", async () => {
    const { store, calls } = setup({ [SPIN]: [json(200, spinResult({ totalWin: 3_000, winTier: "mega", balance: 12_900 }))] });
    await store.getState().spin();
    expect(calls).toContain("celebrate:mega");
  });

  it("restores the balance and shows the error when a spin fails", async () => {
    const { store, calls, balance } = setup({ [SPIN]: [json(429, { code: "rate_limited", message: "Too many requests." })] });
    await store.getState().spin();
    expect(calls).toEqual(["startSpin", "cancelSpin"]);
    expect(balance()).toBe(10_000);
    expect(store.getState()).toMatchObject({ phase: "idle", message: "Too many requests." });
  });

  it("refuses to spin without enough Coins and suggests a lower bet", async () => {
    const { store, requests } = setup({}, 50);
    await store.getState().spin();
    expect(requests).toHaveLength(0);
    expect(store.getState().message).toContain("Try 40");
  });

  it("starts free spins on its own after a trigger and plays them to the end", async () => {
    const b0 = bonusState();
    const b1 = bonusState({ step: 1, totalWin: 200, data: { god: "thor", spinsLeft: 1, spinsPlayed: 1, totalSpins: 2 } });
    const b2 = bonusState({ step: 2, totalWin: 1_400, status: "completed", actions: [], data: { god: "thor", spinsLeft: 0, spinsPlayed: 2, totalSpins: 2 } });
    const { store, calls, requests, balance } = setup({
      [SPIN]: [json(200, spinResult({ bonusTrigger: { kind: "free_spins", positions: [], data: { god: "thor", spins: 2 } }, bonus: b0 }))],
      [ACTION]: [json(200, stepResult(b1, 200, 10_100)), json(200, stepResult(b2, 1_200, 11_300))],
    });
    await store.getState().spin();

    expect(calls.indexOf("bonusIntro")).toBeLessThan(calls.indexOf("bonusStep"));
    expect(requests.filter((r) => r.path === ACTION).map((r) => r.body)).toEqual([
      { action: "spin", step: 0 },
      { action: "spin", step: 1 },
    ]);
    expect(calls).toContain("celebrate:big");
    expect(calls.at(-1)).toBe("bonusEnd");
    expect(store.getState()).toMatchObject({ phase: "idle", bonus: null, lastWin: 1_400 });
    expect(balance()).toBe(11_300);
  });

  it("recovers from a stale step by reloading the bonus", async () => {
    const b0 = bonusState();
    const b1 = bonusState({ step: 1, data: { god: "odin", spinsLeft: 1, spinsPlayed: 1, totalSpins: 2 } });
    const b2 = bonusState({ step: 2, status: "completed", actions: [], totalWin: 0 });
    const { store, requests } = setup({
      [BONUS]: [json(200, b0), json(200, b1)],
      [ACTION]: [json(409, { code: "stale_step", message: "moved on" }), json(200, stepResult(b2, 0, 9_900))],
    });
    await store.getState().resume();
    await vi.waitFor(() => expect(store.getState().phase).toBe("idle"));
    const steps = requests.filter((r) => r.path === ACTION).map((r) => (r.body as { step: number }).step);
    expect(steps).toEqual([0, 1]);
  });

  it("resumes an unfinished bonus after a reload, from the same step", async () => {
    const b = bonusState({ step: 3, totalWin: 500 });
    const done = bonusState({ step: 4, totalWin: 500, status: "completed", actions: [] });
    const { store, calls, requests } = setup({ [BONUS]: [json(200, b)], [ACTION]: [json(200, stepResult(done, 0, 9_900))] });
    await store.getState().resume();
    expect(calls[0]).toBe("resumeBonus");
    await vi.waitFor(() => expect(store.getState().phase).toBe("idle"));
    expect(requests.find((r) => r.path === ACTION)?.body).toEqual({ action: "spin", step: 3 });
    expect(calls.at(-1)).toBe("bonusEnd");
  });

  it("stays in the base game when there is no bonus to resume", async () => {
    const { store, calls } = setup({ [BONUS]: [new Response(null, { status: 204 })] });
    await store.getState().resume();
    expect(calls).toEqual([]);
    expect(store.getState().phase).toBe("idle");
  });

  it("pauses free spins between steps and continues on request", async () => {
    const b0 = bonusState();
    const b1 = bonusState({ step: 1, data: { god: "loki", spinsLeft: 1, spinsPlayed: 1, totalSpins: 2 } });
    const b2 = bonusState({ step: 2, status: "completed", actions: [] });
    const { store, requests } = setup({
      [BONUS]: [json(200, b0)],
      [ACTION]: [json(200, stepResult(b1, 0, 9_900)), json(200, stepResult(b2, 0, 9_900))],
    });
    await store.getState().resume();
    store.getState().pauseBonus();
    await vi.waitFor(() => expect(store.getState().phase).toBe("bonus"));
    expect(requests.filter((r) => r.path === ACTION)).toHaveLength(1);
    expect(store.getState().bonus).toEqual(b1);

    await store.getState().playBonus();
    expect(requests.filter((r) => r.path === ACTION)).toHaveLength(2);
    expect(store.getState().phase).toBe("idle");
  });

  it("autoplays the requested number of spins", async () => {
    const results = Array.from({ length: 3 }, (_, i) => json(200, spinResult({ balance: 9_900 - i * 100 })));
    const { store, requests } = setup({ [SPIN]: results });
    store.getState().startAutoplay({ spins: 3, stopOnBonus: true, stopOnWinX: null, lossLimit: null });
    await vi.waitFor(() => expect(store.getState().autoplay).toBeNull());
    expect(requests.filter((r) => r.path === SPIN)).toHaveLength(3);
    expect(store.getState().message).toBe("Autoplay finished.");
  });

  it("stops autoplay on a bonus when asked to, but still plays the bonus", async () => {
    const { store, requests } = setup({
      [SPIN]: [json(200, spinResult({ bonusTrigger: { kind: "free_spins", positions: [] }, bonus: bonusState() }))],
      [ACTION]: [json(200, stepResult(bonusState({ step: 1, status: "completed", actions: [] }), 0, 9_900))],
    });
    store.getState().startAutoplay({ spins: 10, stopOnBonus: true, stopOnWinX: null, lossLimit: null });
    await vi.waitFor(() => expect(store.getState().phase).toBe("idle"));
    expect(requests.map((r) => r.path)).toEqual([SPIN, ACTION]);
    expect(store.getState()).toMatchObject({ autoplay: null, message: "Autoplay stopped: bonus triggered." });
  });

  it("keeps autoplaying after a bonus when not told to stop", async () => {
    const { store, requests } = setup({
      [SPIN]: [
        json(200, spinResult({ bonusTrigger: { kind: "free_spins", positions: [] }, bonus: bonusState() })),
        json(200, spinResult({ balance: 9_800 })),
      ],
      [ACTION]: [json(200, stepResult(bonusState({ step: 1, status: "completed", actions: [] }), 0, 9_900))],
    });
    store.getState().startAutoplay({ spins: 2, stopOnBonus: false, stopOnWinX: null, lossLimit: null });
    await vi.waitFor(() => expect(store.getState().autoplay).toBeNull());
    expect(requests.map((r) => r.path)).toEqual([SPIN, ACTION, SPIN]);
    expect(store.getState().message).toBe("Autoplay finished.");
  });

  it("does not change the bet while spinning or autoplaying", async () => {
    const { store } = setup({});
    store.getState().changeBet(1);
    expect(store.getState().bet).toBe(200);
    store.setState({ phase: "spinning" });
    store.getState().changeBet(-1);
    expect(store.getState().bet).toBe(200);
  });
});
