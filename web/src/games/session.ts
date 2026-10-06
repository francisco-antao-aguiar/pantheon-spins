import { createStore } from "zustand/vanilla";
import { ApiError, unwrap, type ApiClient, type BonusState, type GameInfo } from "../api/client";
import { affordableBet, autoplayStopReason, STOP_MESSAGES, stepBet, type AutoplayRun, type AutoplaySettings } from "../stores/play";
import type { GameRenderer, PlayOptions, WinTier } from "./types";

export type Phase =
  | "idle" // ready to spin
  | "spinning" // a paid spin is in flight or animating
  | "bonus" // a bonus is waiting to be played (after the intro, or after a reload)
  | "bonusPlaying"; // free spins are playing

export interface SlotSessionDeps {
  api: ApiClient;
  renderer: GameRenderer;
  getBalance(): number;
  setBalance(balance: number): void;
  options(): PlayOptions;
  onBetChange?(bet: number): void;
  /** Waits between automatic actions. Injected so tests run instantly. */
  delay?(ms: number): Promise<void>;
}

export interface SlotSessionState {
  info: GameInfo;
  phase: Phase;
  bet: number;
  bonus: BonusState | null;
  /** Win of the last spin, or the running total of the current bonus. */
  lastWin: number;
  message: string;
  autoplay: AutoplayRun | null;
  bonusPaused: boolean;

  changeBet(dir: 1 | -1): void;
  spin(): Promise<void>;
  startAutoplay(settings: AutoplaySettings): void;
  stopAutoplay(message?: string): void;
  playBonus(): Promise<void>;
  pauseBonus(): void;
  /** Loads an unresolved bonus from the server, restores it and plays it on. */
  resume(): Promise<void>;
  dismissMessage(): void;
}

const CELEBRATED: WinTier[] = ["big", "mega", "epic"];

export function createSlotSession(info: GameInfo, initialBet: number, deps: SlotSessionDeps) {
  const delay = deps.delay ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));
  const path = { params: { path: { gameId: info.id } } };
  const errorText = (err: unknown) => (err instanceof ApiError ? err.message : "Connection problem. Please try again.");

  return createStore<SlotSessionState>()((set, get) => {
    const continueAutoplay = async (last: { win: number; bonusTriggered: boolean }) => {
      const run = get().autoplay;
      if (!run) return;
      const next = { ...run, remaining: run.remaining - 1 };
      const reason = autoplayStopReason(next, { ...last, bet: get().bet, balance: deps.getBalance() });
      if (reason) {
        get().stopAutoplay(STOP_MESSAGES[reason]);
        return;
      }
      set({ autoplay: next });
      await delay(deps.options().turbo ? 150 : 450);
      if (get().autoplay) void get().spin();
    };

    return {
      info,
      phase: "idle",
      bet: initialBet,
      bonus: null,
      lastWin: 0,
      message: "",
      autoplay: null,
      bonusPaused: false,

      changeBet(dir) {
        if (get().phase !== "idle" || get().autoplay) return;
        const bet = stepBet(info.betLevels, get().bet, dir);
        set({ bet });
        deps.onBetChange?.(bet);
      },

      async spin() {
        if (get().phase !== "idle") return;
        const { bet } = get();
        const before = deps.getBalance();
        if (before < bet) {
          const lower = affordableBet(info.betLevels, bet, before);
          set({ message: lower < bet && before >= lower ? `Not enough Coins for this bet. Try ${lower.toLocaleString()}.` : "Not enough Coins to spin." });
          get().stopAutoplay();
          return;
        }
        const opts = deps.options();
        set({ phase: "spinning", lastWin: 0, message: "" });
        deps.setBalance(before - bet); // shown immediately; the server's figure replaces it
        deps.renderer.startSpin(opts);

        let res;
        try {
          res = unwrap(await deps.api.POST("/games/{gameId}/spin", { ...path, body: { bet } }));
        } catch (err) {
          await deps.renderer.cancelSpin();
          deps.setBalance(before);
          set({ phase: "idle", message: errorText(err) });
          get().stopAutoplay();
          if (err instanceof ApiError && err.code === "bonus_in_progress") await get().resume();
          return;
        }

        await deps.renderer.playSpin(res, opts);
        if (CELEBRATED.includes(res.winTier)) await deps.renderer.celebrate(res.totalWin, bet, res.winTier, opts);
        deps.setBalance(res.balance);
        set({ lastWin: res.totalWin });

        if (res.bonusTrigger && res.bonus) {
          await deps.renderer.playBonusIntro(res.bonusTrigger, res.bonus, opts);
          set({ phase: "bonus", bonus: res.bonus, bonusPaused: false });
          // Bonuses have no choices, so free spins start on their own.
          if (get().autoplay?.stopOnBonus) get().stopAutoplay(STOP_MESSAGES.bonus);
          await get().playBonus();
          if (get().autoplay && get().phase === "idle") {
            await continueAutoplay({ win: res.totalWin + get().lastWin, bonusTriggered: true });
          }
          return;
        }
        set({ phase: "idle" });
        await continueAutoplay({ win: res.totalWin, bonusTriggered: false });
      },

      startAutoplay(settings) {
        if (get().phase !== "idle" || settings.spins < 1) return;
        set({ autoplay: { ...settings, remaining: settings.spins, startBalance: deps.getBalance() }, message: "" });
        void get().spin();
      },

      stopAutoplay(message) {
        set((s) => ({ autoplay: null, message: message ?? s.message }));
      },

      async playBonus() {
        if (get().phase !== "bonus" || !get().bonus) return;
        set({ phase: "bonusPlaying", bonusPaused: false });
        const opts = deps.options();
        while (get().bonus?.status === "active" && !get().bonusPaused) {
          const bonus = get().bonus!;
          deps.renderer.startSpin(opts);
          let res;
          try {
            res = unwrap(
              await deps.api.POST("/games/{gameId}/bonus/actions", { ...path, body: { action: "spin", step: bonus.step } }),
            );
          } catch (err) {
            await deps.renderer.cancelSpin();
            if (err instanceof ApiError && (err.code === "stale_step" || err.code === "no_active_bonus")) {
              // Another tab (or a retry) moved the bonus on: reload it and carry on.
              const fresh = await deps.api.GET("/games/{gameId}/bonus", path);
              if (fresh.response.status === 204 || !fresh.data) {
                set({ bonus: null, phase: "idle" });
                return;
              }
              set({ bonus: fresh.data });
              continue;
            }
            set({ phase: "bonus", message: errorText(err) });
            return;
          }
          await deps.renderer.playBonusStep(res, opts);
          if (CELEBRATED.includes(res.winTier)) await deps.renderer.celebrate(res.stepWin, res.state.bet, res.winTier, opts);
          deps.setBalance(res.balance);
          set({ bonus: res.state, lastWin: res.state.totalWin });
          if (res.state.status === "completed") {
            await deps.renderer.playBonusEnd(res.state, opts);
            set({ bonus: null, phase: "idle" });
            return;
          }
          await delay(opts.turbo ? 200 : 600);
        }
        if (get().bonus) set({ phase: "bonus" });
      },

      pauseBonus() {
        set({ bonusPaused: true });
      },

      async resume() {
        try {
          const res = await deps.api.GET("/games/{gameId}/bonus", path);
          if (res.response.status === 200 && res.data) {
            deps.renderer.resumeBonus(res.data);
            set({ bonus: res.data, phase: "bonus", lastWin: res.data.totalWin, bonusPaused: false });
            // Carry on where it stopped, without waiting for the player.
            void get().playBonus();
          }
        } catch (err) {
          set({ message: errorText(err) });
        }
      },

      dismissMessage() {
        set({ message: "" });
      },
    };
  });
}

export type SlotSession = ReturnType<typeof createSlotSession>;
