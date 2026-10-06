import type { Schemas } from "../../api/client";
import type { PlayOptions } from "../types";
import type { ReelGrid } from "./reels";

type SpinOutcome = Schemas["SpinOutcome"];
type SpinStep = Schemas["SpinStep"];

export interface CascadeHooks {
  formatAmount(n: number): string;
  /** Before a step's wins are shown (e.g. update a multiplier meter). */
  beforeWins?(step: SpinStep, index: number): Promise<void> | void;
  /** Plays a step's events (they change the board before its wins). */
  playEvent?(event: Schemas["GameEvent"], step: SpinStep, index: number): Promise<void>;
  /** Plays after a step's cells burst and before the board drops. */
  onClear?(step: SpinStep, index: number): void;
}

/**
 * Presents a stepped outcome on a reel grid: the reels land on the first
 * grid, then each step shows its wins, bursts the cleared cells and tumbles
 * into the next step's grid. Works for single-step spins too.
 */
export async function presentCascades(reels: ReelGrid, outcome: SpinOutcome, opts: PlayOptions, hooks: CascadeHooks) {
  const steps = outcome.steps;
  await reels.stop(steps[0]!.grid, { turbo: opts.turbo, anticipation: outcome.anticipation });
  for (let i = 0; i < steps.length; i++) {
    const step = steps[i]!;
    for (const e of step.events ?? []) await hooks.playEvent?.(e, step, i);
    await hooks.beforeWins?.(step, i);
    if (step.wins.length > 0) await reels.highlightWins(step.wins, { turbo: opts.turbo, formatAmount: hooks.formatAmount });
    const next = steps[i + 1];
    if (step.removed?.length && next) {
      reels.clearHighlight();
      hooks.onClear?.(step, i);
      await reels.explode(step.removed);
      await reels.cascadeTo(next.grid, step.removed);
    }
  }
}
