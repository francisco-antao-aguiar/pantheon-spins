import { Howl, Howler } from "howler";

const BASE = "/audio/";

/** Shared sounds every game can use. */
export type CommonSound = "spin" | "reel-stop" | "anticipation" | "win-small" | "win-big" | "coin" | "click" | "bonus-trigger";

const COMMON: CommonSound[] = ["spin", "reel-stop", "anticipation", "win-small", "win-big", "coin", "click", "bonus-trigger"];

/**
 * Loads and plays sound effects and one music loop at a time. Files are
 * loaded lazily per game; missing files fail silently so audio can never
 * break gameplay.
 */
export class SoundManager {
  private sounds = new Map<string, Howl>();
  private music: Howl | null = null;
  private musicName = "";
  private muted = false;

  constructor(muted = false) {
    this.setMuted(muted);
    for (const name of COMMON) this.load(name);
  }

  /** Loads sounds by path relative to /audio (without extension). */
  load(...names: string[]) {
    for (const name of names) {
      if (this.sounds.has(name)) continue;
      this.sounds.set(name, new Howl({ src: [BASE + name + ".wav"], preload: true, volume: 0.7 }));
    }
  }

  play(name: string, opts: { volume?: number; rate?: number } = {}) {
    const s = this.sounds.get(name);
    if (!s || this.muted) return;
    const id = s.play();
    if (opts.volume !== undefined) s.volume(opts.volume, id);
    if (opts.rate !== undefined) s.rate(opts.rate, id);
  }

  /** Starts a looping music track, cross-fading from the current one. */
  playMusic(name: string, volume = 0.35) {
    if (this.musicName === name) return;
    const prev = this.music;
    if (prev) {
      prev.fade(prev.volume(), 0, 600);
      setTimeout(() => prev.unload(), 700);
    }
    this.musicName = name;
    this.music = new Howl({ src: [BASE + name + ".wav"], loop: true, volume: 0 });
    this.music.play();
    this.music.fade(0, volume, 800);
  }

  stopMusic() {
    this.music?.unload();
    this.music = null;
    this.musicName = "";
  }

  setMuted(muted: boolean) {
    this.muted = muted;
    Howler.mute(muted);
  }

  isMuted() {
    return this.muted;
  }

  destroy() {
    this.stopMusic();
    for (const s of this.sounds.values()) s.unload();
    this.sounds.clear();
  }
}
