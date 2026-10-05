import { describe, expect, it } from "vitest";
import { affordableBet, autoplayStopReason, stepBet, type AutoplayRun } from "./play";

const levels = [20, 40, 100, 200, 400];

describe("stepBet", () => {
  it.each([
    [100, 1, 200],
    [100, -1, 40],
    [400, 1, 400],
    [20, -1, 20],
    [90, 1, 100], // not a level: snaps to the closest
    [1000, -1, 400],
  ] as const)("from %i in direction %i gives %i", (current, dir, want) => {
    expect(stepBet(levels, current, dir)).toBe(want);
  });

  it("keeps the bet when there are no levels", () => {
    expect(stepBet([], 50, 1)).toBe(50);
  });
});

describe("affordableBet", () => {
  it.each([
    [100, 5000, 100], // affordable: unchanged
    [400, 250, 200], // drops to the largest affordable level
    [100, 10, 20], // nothing affordable: smallest level
  ])("bet %i with balance %i gives %i", (current, balance, want) => {
    expect(affordableBet(levels, current, balance)).toBe(want);
  });
});

describe("autoplayStopReason", () => {
  const run = (over: Partial<AutoplayRun> = {}): AutoplayRun => ({
    spins: 10,
    remaining: 5,
    stopOnBonus: true,
    stopOnWinX: 50,
    lossLimit: 1000,
    startBalance: 10_000,
    ...over,
  });
  const last = (over: Partial<{ win: number; bet: number; bonusTriggered: boolean; balance: number }> = {}) => ({
    win: 0,
    bet: 100,
    bonusTriggered: false,
    balance: 9_500,
    ...over,
  });

  it.each([
    ["keeps going", run(), last(), null],
    ["out of spins", run({ remaining: 0 }), last(), "done"],
    ["bonus with stop-on-bonus", run(), last({ bonusTriggered: true }), "bonus"],
    ["bonus without stop-on-bonus", run({ stopOnBonus: false }), last({ bonusTriggered: true }), null],
    ["win at the threshold", run(), last({ win: 5_000 }), "big_win"],
    ["win below the threshold", run(), last({ win: 4_999 }), null],
    ["no win threshold", run({ stopOnWinX: null }), last({ win: 1_000_000 }), null],
    ["loss limit reached", run(), last({ balance: 9_000 }), "loss_limit"],
    ["no loss limit", run({ lossLimit: null }), last({ balance: 100 }), null],
    ["cannot afford the next spin", run({ lossLimit: null }), last({ balance: 99 }), "balance"],
    ["bonus beats every other reason", run({ remaining: 0 }), last({ bonusTriggered: true, win: 10_000 }), "bonus"],
  ] as const)("%s", (_name, r, l, want) => {
    expect(autoplayStopReason(r, l)).toBe(want);
  });
});
