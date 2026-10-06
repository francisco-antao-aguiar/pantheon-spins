import type { Application } from "pixi.js";
import type { BonusState, Schemas, SpinResult } from "../api/client";
import type { SoundManager } from "../audio/sound";

export type BonusStepResult = Schemas["BonusStepResult"];
export type BonusTrigger = Schemas["BonusTrigger"];
export type WinTier = Schemas["WinTier"];
export type BonusAction = { action: string; choice?: string };

/** Bonus kinds whose actions spin the reels (the session starts the reels before the request). */
export const SPIN_BONUSES: Schemas["BonusKind"][] = ["free_spins", "hold_and_win"];

export interface RendererContext {
  app: Application;
  sound: SoundManager;
  /** True when the user prefers reduced motion: no shake, particles or long tweens. */
  reducedMotion: boolean;
}

export interface PlayOptions {
  /** Shorter reel spins and win presentation. */
  turbo: boolean;
}

/**
 * The client half of a slot game. It only animates results the server has
 * already decided; it never computes wins. Each game is a module that exports
 * a factory for its renderer (see registry.ts), mirroring the server's
 * SlotGame packages.
 */
export interface GameRenderer {
  /** Builds the scene on ctx.app.stage and loads assets. */
  init(ctx: RendererContext): Promise<void>;
  /** Lays the scene out for a new canvas size (CSS pixels). */
  resize(width: number, height: number): void;
  /** Starts the reels spinning before the result arrives, so the game feels instant. */
  startSpin(opts: PlayOptions): void;
  /** Stops spinning reels without a result (the request failed). */
  cancelSpin(): Promise<void>;
  /** Lands the reels on the result and presents its wins. */
  playSpin(result: SpinResult, opts: PlayOptions): Promise<void>;
  /** Shows the bonus trigger and intro. */
  playBonusIntro(trigger: BonusTrigger, bonus: BonusState, opts: PlayOptions): Promise<void>;
  /**
   * For bonuses the player interacts with (pick games): resolves with the
   * player's next action, e.g. the urn they tapped. With auto set (autoplay),
   * the renderer chooses on its own. Bonuses without it just send their first
   * offered action ("spin").
   */
  chooseBonusAction?(bonus: BonusState, opts: { auto: boolean }): Promise<BonusAction>;
  /** Animates one bonus action's result. */
  playBonusStep(step: BonusStepResult, opts: PlayOptions): Promise<void>;
  /** Puts the scene into bonus mode without animation (after a reload). */
  resumeBonus(bonus: BonusState): void;
  /** Shows the bonus total and returns to the base game. */
  playBonusEnd(bonus: BonusState, opts: PlayOptions): Promise<void>;
  /** Plays the Big/Mega/Epic celebration for a win. */
  celebrate(win: number, bet: number, tier: WinTier, opts: PlayOptions): Promise<void>;
  /** Skips any running presentation to its end state (tap to skip). */
  skip(): void;
  destroy(): void;
}

/** Lobby and HUD metadata for a game, bundled with its renderer. */
export interface GameModule {
  createRenderer(): GameRenderer;
  /** Short line under the game name on the bonus HUD, e.g. the god leading free spins. */
  describeBonus(bonus: BonusState): string;
}
