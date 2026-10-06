import { Application } from "pixi.js";
import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { useStore } from "zustand";
import { api, unwrap, type SpinResult } from "../api/client";
import { SoundManager } from "../audio/sound";
import { Coins } from "../components/ui";
import { clientGames } from "../games/registry";
import { createSlotSession, type SlotSession } from "../games/session";
import { SPIN_BONUSES, type GameModule, type GameRenderer } from "../games/types";
import { affordableBet, DEFAULT_AUTOPLAY, usePlayPrefs, type AutoplaySettings } from "../stores/play";
import { useSession } from "../stores/session";

const reducedMotion = () => window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;

function readCookie(name: string) {
  return document.cookie
    .split("; ")
    .find((c) => c.startsWith(name + "="))
    ?.slice(name.length + 1);
}

interface Loaded {
  session: SlotSession;
  renderer: GameRenderer;
  module: GameModule;
}

export function GamePage() {
  const { gameId = "" } = useParams();
  const hostRef = useRef<HTMLDivElement>(null);
  const soundRef = useRef<SoundManager | null>(null);
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [error, setError] = useState("");
  const [devtools, setDevtools] = useState(false);
  const muted = usePlayPrefs((s) => s.muted);

  // Build the PixiJS app, renderer and session for this game.
  useEffect(() => {
    let disposed = false;
    const cleanups: (() => void)[] = [];
    const host = hostRef.current!;

    (async () => {
      const client = clientGames[gameId];
      const games = unwrap(await api.GET("/games"));
      const info = games.find((g) => g.id === gameId);
      if (!client || !info) {
        setError("This game does not exist.");
        return;
      }
      const module = await client.load();
      const app = new Application();
      await app.init({
        resizeTo: host,
        backgroundAlpha: 0,
        antialias: true,
        resolution: Math.min(window.devicePixelRatio || 1, 2),
        autoDensity: true,
      });
      cleanups.push(() => app.destroy(true, { children: true }));
      if (disposed) return;
      host.appendChild(app.canvas);

      const sound = new SoundManager(usePlayPrefs.getState().muted);
      soundRef.current = sound;
      cleanups.push(() => sound.destroy());
      const renderer = module.createRenderer();
      await renderer.init({ app, sound, reducedMotion: reducedMotion() });
      cleanups.push(() => renderer.destroy());
      if (disposed) return;

      const resize = () => {
        app.resize();
        renderer.resize(host.clientWidth, host.clientHeight);
      };
      resize();
      const ro = new ResizeObserver(resize);
      ro.observe(host);
      cleanups.push(() => ro.disconnect());

      const prefs = usePlayPrefs.getState();
      const balance = useSession.getState().me?.balance ?? 0;
      const bet = affordableBet(info.betLevels, prefs.bets[gameId] ?? info.defaultBet, balance);
      const session = createSlotSession(info, bet, {
        api,
        renderer,
        getBalance: () => useSession.getState().me?.balance ?? 0,
        setBalance: (b) => useSession.getState().setBalance(b),
        options: () => ({ turbo: usePlayPrefs.getState().turbo }),
        onBetChange: (b) => usePlayPrefs.getState().setBet(gameId, b),
      });
      await session.getState().resume();
      if (disposed) return;
      setLoaded({ session, renderer, module });
    })().catch(() => {
      if (!disposed) setError("Could not load the game. Check your connection and try again.");
    });

    fetch("/api/v1/dev/status").then((r) => setDevtools(r.ok), () => {});

    return () => {
      disposed = true;
      setLoaded(null);
      soundRef.current = null;
      for (const c of cleanups.reverse()) c();
    };
  }, [gameId]);

  useEffect(() => {
    soundRef.current?.setMuted(muted);
  }, [muted]);

  return (
    <div className="fixed inset-0 flex flex-col bg-night-950">
      <TopBar name={loaded?.session.getState().info.name ?? ""} />
      <div className="relative min-h-0 flex-1">
        <div
          ref={hostRef}
          className="absolute inset-0 touch-manipulation"
          onClick={() => {
            // Taps skip animations, except in pick bonuses where taps are the game.
            const { phase, bonus } = loaded?.session.getState() ?? {};
            if (phase === "spinning" || (phase === "bonusPlaying" && bonus && SPIN_BONUSES.includes(bonus.kind))) loaded?.renderer.skip();
          }}
          aria-label="Game screen. Tap to skip animations."
        />
        {!loaded && !error && <div className="absolute inset-0 grid place-items-center text-slate-400">Summoning the reels…</div>}
        {error && (
          <div className="absolute inset-0 grid place-items-center p-6 text-center">
            <div>
              <p className="text-rose-200">{error}</p>
              <Link to="/" className="mt-4 inline-block text-gold-400 underline">
                Back to the lobby
              </Link>
            </div>
          </div>
        )}
        {loaded && <MessageToast session={loaded.session} />}
      </div>
      {loaded && <BonusPanel session={loaded.session} module={loaded.module} />}
      {loaded && <Controls session={loaded.session} renderer={loaded.renderer} devtools={devtools} />}
    </div>
  );
}

function TopBar({ name }: { name: string }) {
  const balance = useSession((s) => s.me?.balance ?? 0);
  const muted = usePlayPrefs((s) => s.muted);
  const setMuted = usePlayPrefs((s) => s.setMuted);
  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-night-700/70 bg-night-950/90 px-3">
      <Link to="/" className="rounded-lg px-2 py-1 text-sm text-slate-300 hover:bg-night-800" aria-label="Back to the lobby">
        ← Lobby
      </Link>
      <h1 className="truncate font-display text-sm font-semibold sm:text-base">{name}</h1>
      <div className="ml-auto flex items-center gap-2">
        <Coins value={balance} className="rounded-full bg-night-800 px-3 py-1 text-sm text-gold-300" />
        <button
          onClick={() => setMuted(!muted)}
          className="grid size-9 place-items-center rounded-full bg-night-800 text-lg hover:bg-night-700"
          aria-pressed={!muted}
          aria-label={muted ? "Sound off" : "Sound on"}
          title={muted ? "Sound off" : "Sound on"}
        >
          {muted ? "🔇" : "🔊"}
        </button>
      </div>
    </header>
  );
}

function BonusPanel({ session, module }: { session: SlotSession; module: GameModule }) {
  const phase = useStore(session, (s) => s.phase);
  const bonus = useStore(session, (s) => s.bonus);
  const paused = useStore(session, (s) => s.bonusPaused);
  if (!bonus) return null;
  const d = bonus.data as { spinsLeft?: number; totalSpins?: number };
  return (
    <div className="flex shrink-0 justify-center border-t border-gold-500/30 bg-night-900/95 px-3 py-2">
      <div className="flex w-full max-w-3xl items-center justify-between gap-3">
        <div className="min-w-0 text-sm">
          <p className="font-semibold text-gold-300">{module.describeBonus(bonus)}</p>
          <p className="text-xs text-slate-400">
            {d.totalSpins !== undefined && (
              <>
                {d.spinsLeft ?? 0} of {d.totalSpins} spins left ·{" "}
              </>
            )}
            won <Coins value={bonus.totalWin} className="text-slate-200" />
          </p>
        </div>
        {phase === "bonus" && (
          <button
            onClick={() => void session.getState().playBonus()}
            className="rounded-xl bg-gradient-to-b from-gold-300 to-gold-600 px-4 py-2 font-semibold text-night-950"
          >
            Continue
          </button>
        )}
        {phase === "bonusPlaying" && SPIN_BONUSES.includes(bonus.kind) && (
          <button onClick={() => session.getState().pauseBonus()} className="rounded-xl border border-night-600 px-3 py-2 text-sm" disabled={paused}>
            {paused ? "Pausing…" : "Pause"}
          </button>
        )}
      </div>
    </div>
  );
}

function MessageToast({ session }: { session: SlotSession }) {
  const message = useStore(session, (s) => s.message);
  useEffect(() => {
    if (!message) return;
    const t = setTimeout(() => session.getState().dismissMessage(), 5000);
    return () => clearTimeout(t);
  }, [message, session]);
  if (!message) return null;
  return (
    <div role="status" className="absolute inset-x-0 top-3 flex justify-center px-3">
      <button onClick={() => session.getState().dismissMessage()} className="rounded-xl bg-night-900/95 px-4 py-2 text-sm text-slate-100 shadow-lg">
        {message}
      </button>
    </div>
  );
}

function Controls({ session, renderer, devtools }: { session: SlotSession; renderer: GameRenderer; devtools: boolean }) {
  const { phase, bet, lastWin, autoplay, info } = useStore(session);
  const turbo = usePlayPrefs((s) => s.turbo);
  const setTurbo = usePlayPrefs((s) => s.setTurbo);
  const [showAuto, setShowAuto] = useState(false);
  const idle = phase === "idle";

  // Space bar spins (or stops autoplay).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== "Space" || (e.target as HTMLElement)?.closest("input, button, select, textarea, dialog")) return;
      e.preventDefault();
      const s = session.getState();
      if (s.autoplay) s.stopAutoplay();
      else if (s.phase === "idle") void s.spin();
      else renderer.skip();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [session, renderer]);

  const forceBonus = async () => {
    const token = readCookie("ps_csrf") ?? unwrap(await api.GET("/auth/csrf")).token;
    const res = await fetch(`/api/v1/dev/force-bonus/${info.id}`, { method: "POST", headers: { "X-CSRF-Token": token } });
    if (!res.ok) {
      session.setState({ message: ((await res.json()) as { message?: string }).message ?? "Could not force a bonus." });
      return;
    }
    const spin = (await res.json()) as SpinResult;
    const opts = { turbo: usePlayPrefs.getState().turbo };
    session.setState({ phase: "spinning" });
    renderer.startSpin(opts);
    await renderer.playSpin(spin, opts);
    useSession.getState().setBalance(spin.balance);
    if (spin.bonusTrigger && spin.bonus) await renderer.playBonusIntro(spin.bonusTrigger, spin.bonus, opts);
    session.setState({ phase: "bonus", bonus: spin.bonus ?? null, lastWin: spin.totalWin });
    void session.getState().playBonus();
  };

  return (
    <footer className="shrink-0 border-t border-night-700/70 bg-night-950/95 px-3 pt-2 pb-[max(0.5rem,env(safe-area-inset-bottom))]">
      <div className="mx-auto grid max-w-3xl grid-cols-[1fr_auto_1fr] items-center gap-2">
        <div className="flex min-w-0 flex-col gap-1 justify-self-start sm:flex-row sm:items-center sm:gap-6">
          <div className="flex min-w-0 flex-col">
            <span className="text-[11px] tracking-wide text-slate-500 uppercase">Win</span>
            <Coins value={lastWin} className={lastWin > 0 ? "text-gold-300" : "text-slate-400"} />
          </div>

        <div className="flex items-center gap-1.5" aria-label="Bet">
          <button
            className="grid size-9 place-items-center rounded-full bg-night-800 text-lg disabled:opacity-40"
            onClick={() => session.getState().changeBet(-1)}
            disabled={!idle || !!autoplay || bet === info.betLevels[0]}
            aria-label="Lower bet"
          >
            −
          </button>
          <div className="min-w-16 text-center">
            <span className="block text-[11px] tracking-wide text-slate-500 uppercase">Bet</span>
            <Coins value={bet} className="text-slate-100" />
          </div>
          <button
            className="grid size-9 place-items-center rounded-full bg-night-800 text-lg disabled:opacity-40"
            onClick={() => session.getState().changeBet(1)}
            disabled={!idle || !!autoplay || bet === info.betLevels.at(-1)}
            aria-label="Raise bet"
          >
            +
          </button>
        </div>
        </div>

        <button
          onClick={() => (autoplay ? session.getState().stopAutoplay() : void session.getState().spin())}
          disabled={!autoplay && !idle}
          className="grid size-16 shrink-0 place-items-center rounded-full bg-gradient-to-b from-gold-300 to-gold-600 font-display text-sm font-bold text-night-950 shadow-lg shadow-gold-600/30 transition active:scale-95 disabled:opacity-50 sm:size-[72px]"
          aria-label={autoplay ? "Stop autoplay" : "Spin"}
        >
          {autoplay ? (
            <span className="leading-tight">
              STOP
              <br />
              <span className="text-xs">{autoplay.remaining}</span>
            </span>
          ) : (
            "SPIN"
          )}
        </button>

        <div className="flex flex-wrap items-center justify-end gap-1.5 justify-self-end">
          <button
            onClick={() => setShowAuto(true)}
            disabled={!idle || !!autoplay}
            className="rounded-xl bg-night-800 px-2.5 py-2 text-xs font-semibold disabled:opacity-40"
          >
            AUTO
          </button>
          <button
            onClick={() => setTurbo(!turbo)}
            aria-pressed={turbo}
            className={`rounded-xl px-2.5 py-2 text-xs font-semibold ${turbo ? "bg-gold-500 text-night-950" : "bg-night-800"}`}
          >
            TURBO
          </button>
          {devtools && (
            <button
              onClick={() => void forceBonus()}
              disabled={!idle}
              className="rounded-xl border border-dashed border-fuchsia-400/60 px-2 py-2 text-[11px] text-fuchsia-200 disabled:opacity-40"
              title="Development only: plays a paid spin that triggers the bonus"
            >
              DEV bonus
            </button>
          )}
        </div>
      </div>
      {showAuto && (
        <AutoplayDialog
          onClose={() => setShowAuto(false)}
          onStart={(s) => {
            setShowAuto(false);
            session.getState().startAutoplay(s);
          }}
        />
      )}
    </footer>
  );
}

function AutoplayDialog({ onClose, onStart }: { onClose: () => void; onStart: (s: AutoplaySettings) => void }) {
  const saved = usePlayPrefs((s) => s.autoplay);
  const save = usePlayPrefs((s) => s.setAutoplay);
  const [s, setS] = useState<AutoplaySettings>(saved ?? DEFAULT_AUTOPLAY);
  const select = "min-h-10 rounded-lg border border-night-600 bg-night-900 px-2";
  return (
    <div className="fixed inset-0 z-20 grid place-items-center bg-black/60 p-4" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="autoplay-title"
        className="w-full max-w-sm rounded-2xl border border-night-700 bg-night-900 p-5"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="autoplay-title" className="mb-4 font-display text-lg font-semibold">
          Autoplay
        </h2>
        <fieldset className="mb-4">
          <legend className="mb-2 text-sm text-slate-300">Number of spins</legend>
          <div className="grid grid-cols-4 gap-2">
            {[10, 25, 50, 100].map((n) => (
              <button
                key={n}
                onClick={() => setS({ ...s, spins: n })}
                aria-pressed={s.spins === n}
                className={`rounded-lg py-2 text-sm font-semibold ${s.spins === n ? "bg-gold-500 text-night-950" : "bg-night-800"}`}
              >
                {n}
              </button>
            ))}
          </div>
        </fieldset>
        <label className="mb-3 flex items-center gap-2 text-sm">
          <input type="checkbox" checked={s.stopOnBonus} onChange={(e) => setS({ ...s, stopOnBonus: e.target.checked })} className="size-4 accent-gold-500" />
          Stop when a bonus triggers
        </label>
        <label className="mb-3 flex items-center justify-between gap-2 text-sm">
          Stop on a single win of
          <select
            className={select}
            value={s.stopOnWinX ?? ""}
            onChange={(e) => setS({ ...s, stopOnWinX: e.target.value ? Number(e.target.value) : null })}
          >
            <option value="">never</option>
            <option value="10">10× bet</option>
            <option value="50">50× bet</option>
            <option value="100">100× bet</option>
          </select>
        </label>
        <label className="mb-5 flex items-center justify-between gap-2 text-sm">
          Stop after losing
          <select className={select} value={s.lossLimit ?? ""} onChange={(e) => setS({ ...s, lossLimit: e.target.value ? Number(e.target.value) : null })}>
            <option value="">no limit</option>
            <option value="1000">1,000 Coins</option>
            <option value="5000">5,000 Coins</option>
            <option value="10000">10,000 Coins</option>
          </select>
        </label>
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="rounded-xl px-4 py-2 text-sm text-slate-300">
            Cancel
          </button>
          <button
            onClick={() => {
              save(s);
              onStart(s);
            }}
            className="rounded-xl bg-gradient-to-b from-gold-300 to-gold-600 px-4 py-2 text-sm font-semibold text-night-950"
          >
            Start {s.spins} spins
          </button>
        </div>
      </div>
    </div>
  );
}
