import type { Ticker } from "pixi.js";

export type Ease = (t: number) => number;

export const ease = {
  linear: (t: number) => t,
  inCubic: (t: number) => t * t * t,
  outCubic: (t: number) => 1 - Math.pow(1 - t, 3),
  inOutSine: (t: number) => -(Math.cos(Math.PI * t) - 1) / 2,
  outBack: (t: number) => {
    const c1 = 1.70158;
    const c3 = c1 + 1;
    return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2);
  },
  outElastic: (t: number) =>
    t === 0 || t === 1 ? t : Math.pow(2, -10 * t) * Math.sin((t * 10 - 0.75) * ((2 * Math.PI) / 3)) + 1,
} satisfies Record<string, Ease>;

type Job = { update: (dtMs: number) => boolean; finish: () => void };

/** Keys of T whose values are numbers (what a tween can animate). */
type NumericKeys<T> = { [K in keyof T]-?: T[K] extends number ? K : never }[keyof T];

/**
 * Minimal tween/timer engine on the Pixi ticker. `speed` scales every
 * duration (turbo), and `skip()` jumps every running job to its end so a
 * player can tap through a presentation.
 */
export class Tweener {
  speed = 1;
  /** While set, every new job completes at once (the player skipped). */
  skipping = false;
  private jobs = new Set<Job>();

  constructor(private ticker: Ticker) {
    ticker.add(this.tick, this);
  }

  private tick() {
    const dt = this.ticker.deltaMS * this.speed;
    for (const job of [...this.jobs]) {
      if (job.update(dt)) this.jobs.delete(job);
    }
  }

  /** Animates numeric properties of target to the given values. */
  to<T extends object>(target: T, props: Partial<Record<NumericKeys<T>, number>>, ms: number, easing: Ease = ease.outCubic): Promise<void> {
    const keys = Object.keys(props) as NumericKeys<T>[];
    const from = keys.map((k) => target[k] as unknown as number);
    const to = keys.map((k) => props[k] as number);
    return this.run(ms, (p) => {
      const e = easing(p);
      keys.forEach((k, i) => {
        (target[k] as unknown as number) = from[i]! + (to[i]! - from[i]!) * e;
      });
    });
  }

  /** Calls fn(progress 0..1) every frame for ms milliseconds. */
  run(ms: number, fn: (p: number) => void): Promise<void> {
    return new Promise((resolve) => {
      let elapsed = 0;
      const finish = () => {
        fn(1);
        resolve();
      };
      if (ms <= 0 || this.skipping) {
        finish();
        return;
      }
      this.jobs.add({
        update: (dt) => {
          elapsed += dt;
          if (elapsed >= ms) {
            finish();
            return true;
          }
          fn(elapsed / ms);
          return false;
        },
        finish,
      });
    });
  }

  wait(ms: number): Promise<void> {
    return this.run(ms, () => {});
  }

  /** Finishes every running job, and any started until resetSkip(). */
  skip() {
    this.skipping = true;
    const jobs = [...this.jobs];
    this.jobs.clear();
    for (const j of jobs) j.finish();
  }

  resetSkip() {
    this.skipping = false;
  }

  destroy() {
    this.ticker.remove(this.tick, this);
    this.jobs.clear();
  }
}
