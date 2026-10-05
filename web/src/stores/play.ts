import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

/** Steps the bet to the next level up (dir 1) or down (dir -1), clamped. */
export function stepBet(levels: number[], current: number, dir: 1 | -1): number {
  if (levels.length === 0) return current;
  let i = levels.indexOf(current);
  if (i === -1) {
    // Not a level (e.g. config changed): snap to the closest level.
    i = levels.reduce((best, l, j) => (Math.abs(l - current) < Math.abs(levels[best]! - current) ? j : best), 0);
    return levels[i]!;
  }
  return levels[Math.min(levels.length - 1, Math.max(0, i + dir))]!;
}

/** The largest bet level the balance can cover, or the smallest level. */
export function affordableBet(levels: number[], current: number, balance: number): number {
  if (balance >= current) return current;
  const ok = levels.filter((l) => l <= balance);
  return ok.length > 0 ? ok[ok.length - 1]! : (levels[0] ?? current);
}

export interface AutoplaySettings {
  spins: number;
  stopOnBonus: boolean;
  /** Stop when a single spin wins at least this many times the bet (null: never). */
  stopOnWinX: number | null;
  /** Stop when the net loss since autoplay started reaches this many Coins (null: never). */
  lossLimit: number | null;
}

export interface AutoplayRun extends AutoplaySettings {
  remaining: number;
  startBalance: number;
}

export type StopReason = "done" | "bonus" | "big_win" | "loss_limit" | "balance";

export const STOP_MESSAGES: Record<StopReason, string> = {
  done: "Autoplay finished.",
  bonus: "Autoplay stopped: bonus triggered.",
  big_win: "Autoplay stopped: big win!",
  loss_limit: "Autoplay stopped: loss limit reached.",
  balance: "Autoplay stopped: not enough Coins for the next spin.",
};

/** Decides whether autoplay stops after a spin. remaining is already decremented. */
export function autoplayStopReason(
  run: AutoplayRun,
  last: { win: number; bet: number; bonusTriggered: boolean; balance: number },
): StopReason | null {
  if (run.stopOnBonus && last.bonusTriggered) return "bonus";
  if (run.stopOnWinX !== null && last.win >= run.stopOnWinX * last.bet) return "big_win";
  if (run.lossLimit !== null && run.startBalance - last.balance >= run.lossLimit) return "loss_limit";
  if (last.balance < last.bet) return "balance";
  if (run.remaining <= 0) return "done";
  return null;
}

interface PlayPrefs {
  turbo: boolean;
  muted: boolean;
  /** Last bet per game. */
  bets: Record<string, number>;
  autoplay: AutoplaySettings;
  setTurbo(on: boolean): void;
  setMuted(on: boolean): void;
  setBet(gameId: string, bet: number): void;
  setAutoplay(s: AutoplaySettings): void;
}

export const DEFAULT_AUTOPLAY: AutoplaySettings = { spins: 25, stopOnBonus: true, stopOnWinX: 50, lossLimit: null };

/** Per-viewer preferences, remembered in this browser. */
export const usePlayPrefs = create<PlayPrefs>()(
  persist(
    (set) => ({
      turbo: false,
      muted: false,
      bets: {},
      autoplay: DEFAULT_AUTOPLAY,
      setTurbo: (turbo) => set({ turbo }),
      setMuted: (muted) => set({ muted }),
      setBet: (gameId, bet) => set((s) => ({ bets: { ...s.bets, [gameId]: bet } })),
      setAutoplay: (autoplay) => set({ autoplay }),
    }),
    {
      name: "pantheon-play-prefs",
      storage: createJSONStorage(() => {
        try {
          return localStorage;
        } catch {
          return sessionStorage;
        }
      }),
    },
  ),
);
