import { Assets, Container, FillGradient, Graphics, Sprite, Text, type Texture, type Ticker } from "pixi.js";
import type { BonusState, Schemas, SpinResult } from "../../api/client";
import { Effects } from "../engine/effects";
import { ReelGrid } from "../engine/reels";
import { ease, Tweener } from "../engine/tween";
import type { BonusStepResult, BonusTrigger, GameRenderer, PlayOptions, RendererContext, WinTier } from "../types";

type GameEvent = Schemas["GameEvent"];
type SpinOutcome = Schemas["SpinOutcome"];

// Art manifest: symbol ID → file. Swap files (or this map, for an atlas) to
// replace the placeholder art.
const art = import.meta.glob("./art/*.svg", { eager: true, query: "?url", import: "default" }) as Record<string, string>;
const artUrl = (name: string) => art[`./art/${name}.svg`]!;

const SYMBOLS = [
  "fehu", "uruz", "thurisaz", "ansuz", "shield", "axe", "horn", "valkyrie",
  "wild", "bonus_odin", "bonus_thor", "bonus_loki",
];
const BLUR = SYMBOLS.slice(0, 9);
const GOD_NAME: Record<string, string> = { odin: "Odin", thor: "Thor", loki: "Loki" };
const GOD_COLOR: Record<string, number> = { odin: 0x9fd0ff, thor: 0xffd36b, loki: 0x7dffb0 };

const CELL = 150;
const REELS = 5;
const ROWS = 4;
const PAD = 22;

const coins = (n: number) => n.toLocaleString();

export class ValhallaRenderer implements GameRenderer {
  private ctx!: RendererContext;
  private t!: Tweener;
  private fx!: Effects;
  private textures = new Map<string, Texture>();
  private raven!: Texture;

  private world = new Container(); // shakes
  private sky = new Graphics();
  private aurora = new Graphics();
  private mountains = new Graphics();
  private board = new Container();
  private frame = new Graphics();
  private reels!: ReelGrid;
  private fxLayer = new Container(); // ravens, bolts, swirls (board coordinates)
  private hud = new Text({ text: "", style: { fontFamily: "Cinzel, Georgia, serif", fontSize: 30, fontWeight: "700", fill: 0xffe08a, stroke: { color: 0x120a00, width: 6 } } });
  private overlay = new Container();

  private width = 1280;
  private height = 720;
  private freeMode = false;
  private god = "";
  private auroraPhase = 0;

  async init(ctx: RendererContext) {
    this.ctx = ctx;
    const { app } = ctx;
    this.t = new Tweener(app.ticker);
    ctx.sound.load("valhalla/raven", "valhalla/lightning", "valhalla/loki", "valhalla/horn");

    const loaded = await Promise.all(
      [...SYMBOLS, "raven"].map((id) => Assets.load<Texture>({ src: artUrl(id), data: { resolution: 1.5 } }).then((tex) => [id, tex] as const)),
    );
    for (const [id, tex] of loaded) this.textures.set(id, tex);
    this.raven = this.textures.get("raven")!;

    this.reels = new ReelGrid({
      heights: Array(REELS).fill(ROWS),
      cell: CELL,
      textures: this.textures,
      blurSymbols: BLUR,
      tweener: this.t,
      ticker: app.ticker,
      sound: ctx.sound,
      reducedMotion: ctx.reducedMotion,
      accent: 0xffd36b,
    });
    this.reels.x = PAD;
    this.reels.y = PAD;
    this.reels.setGrid(this.randomGrid());
    this.hud.anchor.set(0.5, 1);
    this.hud.x = PAD + (REELS * CELL) / 2;
    this.hud.y = -10;
    this.board.addChild(this.frame, this.reels, this.fxLayer, this.hud);
    this.drawFrame();

    this.world.addChild(this.sky, this.aurora, this.mountains, this.board);
    app.stage.addChild(this.world, this.overlay);
    this.fx = new Effects({
      app,
      tweener: this.t,
      sound: ctx.sound,
      reducedMotion: ctx.reducedMotion,
      shakeTarget: this.world,
      overlay: this.overlay,
      accent: 0xffd36b,
    });
    app.ticker.add(this.animateAurora, this);
    ctx.sound.playMusic("valhalla/music-base");
  }

  private randomGrid() {
    return Array.from({ length: REELS }, () => Array.from({ length: ROWS }, () => BLUR[Math.floor(Math.random() * 8)]!));
  }

  // ---------- layout & scenery ----------

  resize(width: number, height: number) {
    this.width = width;
    this.height = height;
    this.drawSky();
    this.fx.resize(width, height);
    const bw = REELS * CELL + PAD * 2;
    const bh = ROWS * CELL + PAD * 2 + 50; // room for the HUD line above
    const scale = Math.min((width * 0.96) / bw, (height * 0.94) / bh);
    this.board.scale.set(scale);
    this.board.x = (width - bw * scale) / 2;
    this.board.y = (height - bh * scale) / 2 + 50 * scale;
  }

  private drawSky() {
    const { width: w, height: h } = this;
    const stops = this.freeMode
      ? [
          { offset: 0, color: 0x1a0505 },
          { offset: 0.6, color: 0x5c1410 },
          { offset: 1, color: 0x2a0d24 },
        ]
      : [
          { offset: 0, color: 0x070d22 },
          { offset: 0.6, color: 0x163066 },
          { offset: 1, color: 0x2a2150 },
        ];
    const grad = new FillGradient({ type: "linear", start: { x: 0, y: 0 }, end: { x: 0, y: 1 }, colorStops: stops, textureSpace: "local" });
    this.sky.clear().rect(0, 0, w, h).fill(grad);
    // Stars.
    for (let i = 0; i < 70; i++) {
      const x = (Math.sin(i * 12.9898) * 43758.5453) % 1;
      const y = (Math.sin(i * 78.233) * 12345.678) % 1;
      this.sky.circle(Math.abs(x) * w, Math.abs(y) * h * 0.55, i % 7 === 0 ? 1.8 : 1).fill({ color: 0xffffff, alpha: 0.6 });
    }
    // Mountains.
    const m = this.mountains.clear();
    const peaks = [0, 0.78, 0.1, 0.6, 0.22, 0.72, 0.36, 0.55, 0.5, 0.68, 0.63, 0.5, 0.78, 0.66, 0.9, 0.58, 1, 0.7];
    m.moveTo(0, h);
    for (let i = 0; i < peaks.length; i += 2) m.lineTo(peaks[i]! * w, peaks[i + 1]! * h);
    m.lineTo(w, h).closePath().fill(this.freeMode ? 0x1c0606 : 0x0d1226);
  }

  private animateAurora(ticker: Ticker) {
    if (this.ctx.reducedMotion && this.auroraPhase > 0) return;
    this.auroraPhase += ticker.deltaMS / 4000;
    const { width: w, height: h } = this;
    const g = this.aurora.clear();
    const colors = this.freeMode ? [0xff5a2a, 0xff2a5a] : [0x5bffb0, 0x9d7bff];
    colors.forEach((color, band) => {
      const base = h * (0.18 + band * 0.09);
      g.moveTo(0, base);
      for (let x = 0; x <= w; x += w / 24) {
        g.lineTo(x, base + Math.sin(x / (w / 3) + this.auroraPhase * (band ? 1.3 : 1) * Math.PI * 2) * h * 0.05);
      }
      for (let x = w; x >= 0; x -= w / 24) {
        g.lineTo(x, base + h * 0.07 + Math.sin(x / (w / 2.5) + this.auroraPhase * Math.PI * 2) * h * 0.04);
      }
      g.closePath().fill({ color, alpha: 0.13 });
    });
  }

  private drawFrame() {
    const w = REELS * CELL + PAD * 2;
    const h = ROWS * CELL + PAD * 2;
    const gold = new FillGradient({
      type: "linear",
      start: { x: 0, y: 0 },
      end: { x: 0, y: 1 },
      colorStops: [
        { offset: 0, color: 0xfff0b8 },
        { offset: 0.5, color: 0xe3a937 },
        { offset: 1, color: 0x8a5a12 },
      ],
      textureSpace: "local",
    });
    this.frame
      .clear()
      .roundRect(-6, -6, w + 12, h + 12, 30)
      .fill(gold)
      .roundRect(4, 4, w - 8, h - 8, 22)
      .fill(this.freeMode ? 0x1f0707 : 0x0a0f22)
      .roundRect(4, 4, w - 8, h - 8, 22)
      .stroke({ color: 0x000000, width: 3, alpha: 0.5 });
    for (let r = 1; r < REELS; r++) {
      this.frame.moveTo(PAD + r * CELL, PAD + 8).lineTo(PAD + r * CELL, PAD + ROWS * CELL - 8).stroke({ color: 0xc8a24a, width: 2, alpha: 0.25 });
    }
  }

  private setFreeMode(on: boolean, god = "") {
    this.freeMode = on;
    this.god = god;
    this.drawSky();
    this.drawFrame();
    this.ctx.sound.playMusic(on ? "valhalla/music-free" : "valhalla/music-base");
    if (!on) this.hud.text = "";
  }

  private setHud(bonus: BonusState) {
    const d = bonus.data as { god?: string; spinsPlayed?: number; totalSpins?: number };
    const god = GOD_NAME[d.god ?? ""] ?? "";
    this.hud.text = `${god.toUpperCase()} · FREE SPIN ${Math.min((d.spinsPlayed ?? 0) + (bonus.status === "active" ? 1 : 0), d.totalSpins ?? 0)} OF ${d.totalSpins ?? 0}`;
    this.hud.style.fill = GOD_COLOR[d.god ?? ""] ?? 0xffe08a;
  }

  // ---------- spins ----------

  startSpin(opts: PlayOptions) {
    this.t.resetSkip();
    this.t.speed = opts.turbo ? 1.8 : 1;
    this.reels.startSpin();
    this.ctx.sound.play("spin", { volume: 0.4 });
  }

  async cancelSpin() {
    await this.reels.stop(this.reels.getGrid(), { turbo: true });
  }

  async playSpin(result: SpinResult, opts: PlayOptions) {
    await this.playOutcome(result.outcome, opts);
  }

  private async playOutcome(outcome: SpinOutcome, opts: PlayOptions) {
    const step = outcome.steps[0]!;
    await this.reels.stop(step.grid, { turbo: opts.turbo, anticipation: outcome.anticipation });
    for (const e of step.events ?? []) await this.playEvent(e, opts);
    if (step.wins.length > 0) {
      await this.reels.highlightWins(step.wins, { turbo: opts.turbo, formatAmount: coins });
    }
    for (const e of outcome.events ?? []) await this.playEvent(e, opts);
  }

  private async playEvent(e: GameEvent, opts: PlayOptions) {
    switch (e.type) {
      case "raven_wilds":
        return this.ravens(e.positions ?? [], opts);
      case "lightning":
        return this.lightning(Number(e.value ?? 1));
      case "loki_transform":
        return this.lokiTransform(e.positions ?? [], e.symbol ?? "");
      case "retrigger":
        this.ctx.sound.play("bonus-trigger");
        return this.fx.banner(`+${e.value} FREE SPINS`, { color: GOD_COLOR[this.god], ms: opts.turbo ? 600 : 1200 });
      case "max_win":
        return this.fx.banner("MAX WIN", { sub: `${e.value}× bet reached`, color: 0xff4fd8, ms: 1600 });
    }
  }

  private async ravens(positions: Schemas["Position"][], opts: PlayOptions) {
    this.ctx.sound.play("valhalla/raven");
    await Promise.all(
      positions.map(async (p, i) => {
        const target = this.reels.cellCenter(p);
        const bird = new Sprite(this.raven);
        bird.anchor.set(0.5);
        bird.scale.set(0.9);
        bird.x = -160 - i * 60;
        bird.y = -120;
        this.fxLayer.addChild(bird);
        await this.t.wait(i * (opts.turbo ? 80 : 180));
        const to = { x: target.x + PAD, y: target.y + PAD };
        if (!this.ctx.reducedMotion) {
          await this.t.to(bird, to, opts.turbo ? 350 : 650, ease.inOutSine);
        }
        await this.reels.setSymbol(p, "wild");
        await this.t.to(bird, { alpha: 0, y: to.y - 80 }, 300);
        bird.destroy();
      }),
    );
  }

  private async lightning(mult: number) {
    this.ctx.sound.play("valhalla/lightning");
    const w = REELS * CELL + PAD * 2;
    const bolt = new Graphics();
    let x = w * (0.3 + Math.random() * 0.4);
    let y = -200;
    bolt.moveTo(x, y);
    while (y < ROWS * CELL * 0.8) {
      x += (Math.random() - 0.5) * 120;
      y += 50 + Math.random() * 40;
      bolt.lineTo(x, y);
    }
    bolt.stroke({ color: 0xfff27a, width: 14, alpha: 0.5 }).stroke({ color: 0xffffff, width: 5 });
    this.fxLayer.addChild(bolt);
    void this.fx.flash(0xfff6c0, 280);
    void this.fx.shake(10, 350);
    const label = new Text({
      text: `×${mult}`,
      style: { fontFamily: "Cinzel, Georgia, serif", fontSize: 120, fontWeight: "700", fill: 0xffe08a, stroke: { color: 0x3a1600, width: 10 } },
    });
    label.anchor.set(0.5);
    label.x = w / 2;
    label.y = (ROWS * CELL) / 2 + PAD;
    label.scale.set(this.ctx.reducedMotion ? 1 : 0.2);
    this.fxLayer.addChild(label);
    await this.t.to(label.scale, { x: 1, y: 1 }, 450, ease.outBack);
    await this.t.to(bolt, { alpha: 0 }, 250);
    bolt.destroy();
    await this.t.wait(350);
    await this.t.to(label, { alpha: 0, y: label.y - 60 }, 350);
    label.destroy();
  }

  private async lokiTransform(positions: Schemas["Position"][], symbol: string) {
    this.ctx.sound.play("valhalla/loki");
    await Promise.all(
      positions.map(async (p, i) => {
        const c = this.reels.cellCenter(p);
        const ring = new Graphics().circle(0, 0, CELL * 0.45).stroke({ color: 0x7dffb0, width: 8 });
        ring.x = c.x + PAD;
        ring.y = c.y + PAD;
        ring.scale.set(0.1);
        this.fxLayer.addChild(ring);
        await this.t.wait(i * 70);
        await this.t.run(380, (q) => {
          ring.scale.set(0.1 + q * 1.1);
          ring.rotation = q * Math.PI * 2;
        });
        await this.reels.setSymbol(p, symbol);
        await this.t.to(ring, { alpha: 0 }, 200);
        ring.destroy();
      }),
    );
  }

  // ---------- bonus ----------

  async playBonusIntro(trigger: BonusTrigger, bonus: BonusState, opts: PlayOptions) {
    const data = (trigger.data ?? {}) as { god?: string; spins?: number; votes?: Record<string, number> };
    const god = data.god ?? "";
    this.ctx.sound.play("bonus-trigger");
    await this.reels.highlightWins([{ symbol: "bonus", amount: 0, positions: trigger.positions }], { turbo: opts.turbo, formatAmount: () => "" });
    this.reels.clearHighlight();
    this.ctx.sound.play("valhalla/horn");
    const votes = data.votes ?? {};
    const tally = Object.entries(votes)
      .filter(([, n]) => n > 0)
      .map(([g, n]) => `${GOD_NAME[g]} ${n}`)
      .join(" · ");
    this.setFreeMode(true, god);
    this.setHud(bonus);
    await this.fx.banner(`${(GOD_NAME[god] ?? "The gods").toUpperCase()} LEADS RAGNARÖK`, {
      sub: `${data.spins ?? ""} free spins  ·  ${tally}`,
      color: GOD_COLOR[god],
      ms: opts.turbo ? 1200 : 2200,
    });
  }

  async playBonusStep(step: BonusStepResult, opts: PlayOptions) {
    if (step.outcome) await this.playOutcome(step.outcome, opts);
    this.setHud(step.state);
  }

  resumeBonus(bonus: BonusState) {
    const god = String((bonus.data as { god?: string }).god ?? "");
    this.setFreeMode(true, god);
    this.setHud(bonus);
  }

  async playBonusEnd(bonus: BonusState, opts: PlayOptions) {
    await this.fx.banner("RAGNARÖK COMPLETE", {
      sub: `You won ${coins(bonus.totalWin)} Coins`,
      color: GOD_COLOR[this.god],
      ms: opts.turbo ? 1200 : 2200,
    });
    if (bonus.totalWin > 0) this.fx.coinBurst(this.width / 2, this.height * 0.8, 40);
    this.setFreeMode(false);
  }

  async celebrate(win: number, _bet: number, tier: WinTier, opts: PlayOptions) {
    await this.fx.celebrate(win, tier, coins, opts);
  }

  skip() {
    this.t.skip();
  }

  destroy() {
    this.ctx.app.ticker.remove(this.animateAurora, this);
    this.ctx.sound.stopMusic();
    this.fx.destroy();
    this.t.destroy();
    this.reels.destroy();
    this.world.destroy({ children: true });
    this.overlay.destroy({ children: true });
  }
}
