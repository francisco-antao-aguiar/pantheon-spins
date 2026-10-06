import { Assets, Container, FillGradient, Graphics, Sprite, Text, type Texture, type Ticker } from "pixi.js";
import type { BonusState, SpinResult } from "../../api/client";
import { Effects } from "../engine/effects";
import { presentCascades } from "../engine/present";
import { ReelGrid } from "../engine/reels";
import { ease, Tweener } from "../engine/tween";
import type { BonusAction, BonusStepResult, BonusTrigger, GameRenderer, PlayOptions, RendererContext, WinTier } from "../types";

// Art manifest: ID → file. Swap the files (or this map, for an atlas) for final art.
const art = import.meta.glob("./art/*.svg", { eager: true, query: "?url", import: "default" }) as Record<string, string>;
const artUrl = (name: string) => art[`./art/${name}.svg`]!;

const SYMBOLS = ["lotus", "ankh", "djed", "feather", "jar", "cat", "falcon", "mask", "eye"];
const URN_ART = ["urn", "treasure", "passage", "curse", "pharaoh"];
const BLUR = SYMBOLS.slice(0, 8);

const CELL = 120;
const REELS = 6;
const ROWS = 5;
const PAD = 20;
const TOP = 64; // room above the board for the multiplier meter

const GOLD = 0xffd36b;
const coins = (n: number) => n.toLocaleString();

interface PublicUrn {
  kind: "hidden" | "treasure" | "passage" | "curse" | "pharaoh";
  value?: number;
  revealed?: boolean;
}

interface TombData {
  chamber: number;
  chambers: number;
  multipliers: number[];
  urns: PublicUrn[][];
  ending?: string;
}

export class EyeOfRaRenderer implements GameRenderer {
  private ctx!: RendererContext;
  private t!: Tweener;
  private fx!: Effects;
  private textures = new Map<string, Texture>();

  private world = new Container();
  private sky = new Graphics();
  private glow = new Graphics();
  private dunes = new Graphics();
  private board = new Container();
  private frame = new Graphics();
  private reels!: ReelGrid;
  private meter = new Text({ text: "", style: { fontFamily: "Cinzel, Georgia, serif", fontSize: 40, fontWeight: "700", fill: GOLD, stroke: { color: 0x2a1600, width: 7 } } });
  private overlay = new Container();

  // Tomb Explorer.
  private tomb = new Container();
  private tombTitle = new Text({ text: "", style: { fontFamily: "Cinzel, Georgia, serif", fontSize: 44, fontWeight: "700", fill: GOLD, stroke: { color: 0x1a0d00, width: 7 } } });
  private tombSub = new Text({ text: "", style: { fontFamily: "Inter, sans-serif", fontSize: 24, fill: 0xf3e2c0 } });
  private urnLayer = new Container();
  private urnSprites: Sprite[] = [];
  private urnLabels: Text[] = [];
  private pickResolve: ((a: BonusAction) => void) | null = null;
  private pickChoices = new Set<string>();
  private autoTimer: ReturnType<typeof setTimeout> | null = null;
  private tombData: TombData | null = null;
  private collected = 0;

  private width = 1280;
  private height = 720;
  private inTomb = false;
  private phase = 0;

  async init(ctx: RendererContext) {
    this.ctx = ctx;
    const { app } = ctx;
    this.t = new Tweener(app.ticker);
    ctx.sound.load("eyeofra/crumble", "eyeofra/urn-open", "eyeofra/treasure", "eyeofra/passage", "eyeofra/curse");

    const loaded = await Promise.all(
      [...SYMBOLS, ...URN_ART].map((id) => Assets.load<Texture>({ src: artUrl(id), data: { resolution: 1.5 } }).then((tex) => [id, tex] as const)),
    );
    for (const [id, tex] of loaded) this.textures.set(id, tex);

    this.reels = new ReelGrid({
      heights: Array(REELS).fill(ROWS),
      cell: CELL,
      textures: this.textures,
      blurSymbols: BLUR,
      tweener: this.t,
      ticker: app.ticker,
      sound: ctx.sound,
      reducedMotion: ctx.reducedMotion,
      accent: GOLD,
    });
    this.reels.x = PAD;
    this.reels.y = PAD + TOP;
    this.reels.setGrid(Array.from({ length: REELS }, () => Array.from({ length: ROWS }, () => BLUR[Math.floor(Math.random() * BLUR.length)]!)));
    this.meter.anchor.set(0.5);
    this.meter.x = PAD + (REELS * CELL) / 2;
    this.meter.y = TOP / 2;
    this.board.addChild(this.frame, this.reels, this.meter);
    this.drawFrame();

    this.tombTitle.anchor.set(0.5, 0);
    this.tombSub.anchor.set(0.5, 0);
    this.tomb.addChild(this.tombTitle, this.tombSub, this.urnLayer);
    this.tomb.visible = false;

    this.world.addChild(this.sky, this.glow, this.dunes, this.board, this.tomb);
    app.stage.addChild(this.world, this.overlay);
    this.fx = new Effects({ app, tweener: this.t, sound: ctx.sound, reducedMotion: ctx.reducedMotion, shakeTarget: this.world, overlay: this.overlay, accent: GOLD });
    app.ticker.add(this.animate, this);
    window.addEventListener("keydown", this.onKey);
    ctx.sound.playMusic("eyeofra/music-base");
  }

  /** Number keys 1–9 open the matching urn (keyboard play and accessibility). */
  private onKey = (e: KeyboardEvent) => {
    if (!this.pickResolve || !/^[1-9]$/.test(e.key)) return;
    e.preventDefault();
    this.tap(Number(e.key) - 1);
  };

  // ---------- layout & scenery ----------

  resize(width: number, height: number) {
    this.width = width;
    this.height = height;
    this.fx.resize(width, height);
    this.drawScenery();
    const bw = REELS * CELL + PAD * 2;
    const bh = ROWS * CELL + PAD * 2 + TOP;
    const scale = Math.min((width * 0.96) / bw, (height * 0.95) / bh);
    this.board.scale.set(scale);
    this.board.x = (width - bw * scale) / 2;
    this.board.y = (height - bh * scale) / 2;
    this.layoutTomb();
  }

  private drawScenery() {
    const { width: w, height: h } = this;
    const stops = this.inTomb
      ? [
          { offset: 0, color: 0x1a0f06 },
          { offset: 1, color: 0x3a2410 },
        ]
      : [
          { offset: 0, color: 0x0e0820 },
          { offset: 0.55, color: 0x4a1e0e },
          { offset: 1, color: 0xb05a18 },
        ];
    this.sky.clear().rect(0, 0, w, h).fill(new FillGradient({ type: "linear", start: { x: 0, y: 0 }, end: { x: 0, y: 1 }, colorStops: stops, textureSpace: "local" }));
    const d = this.dunes.clear();
    if (this.inTomb) {
      // Sandstone blocks.
      for (let y = 0; y < h; y += 60) {
        for (let x = (y / 60) % 2 ? -50 : 0; x < w; x += 100) {
          d.rect(x + 2, y + 2, 96, 56).stroke({ color: 0x000000, width: 3, alpha: 0.25 });
        }
      }
      return;
    }
    d.moveTo(0, h).lineTo(w * 0.12, h * 0.72).lineTo(w * 0.3, h).closePath().fill(0x5a3412);
    d.moveTo(w * 0.62, h).lineTo(w * 0.82, h * 0.6).lineTo(w * 1.02, h).closePath().fill(0x6a3e16);
    d.moveTo(w * 0.82, h * 0.6).lineTo(w * 1.02, h).lineTo(w * 0.9, h).closePath().fill(0x4a2a0e);
  }

  private animate(ticker: Ticker) {
    this.phase += ticker.deltaMS / 1000;
    const g = this.glow.clear();
    const { width: w, height: h } = this;
    if (this.inTomb) {
      // Torch flicker in the corners.
      const f = this.ctx.reducedMotion ? 1 : 0.85 + 0.15 * Math.sin(this.phase * 9) * Math.sin(this.phase * 4.3);
      for (const x of [w * 0.06, w * 0.94]) g.circle(x, h * 0.2, h * 0.28 * f).fill({ color: 0xff9d3c, alpha: 0.12 });
      return;
    }
    const pulse = this.ctx.reducedMotion ? 1 : 1 + 0.04 * Math.sin(this.phase * 1.2);
    g.circle(w / 2, h * 0.62, Math.min(w, h) * 0.45 * pulse).fill({ color: 0xffb347, alpha: 0.1 });
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
    this.frame.clear().roundRect(-6, TOP - 6, w + 12, h + 12, 18).fill(gold).roundRect(4, TOP + 4, w - 8, h - 8, 12).fill(0x1a1006);
  }

  private setMeter(m: number) {
    this.meter.text = m > 1 ? `CASCADE ×${m}` : "";
  }

  // ---------- base game ----------

  startSpin(opts: PlayOptions) {
    this.t.resetSkip();
    this.t.speed = opts.turbo ? 1.8 : 1;
    this.setMeter(1);
    this.reels.startSpin();
    this.ctx.sound.play("spin", { volume: 0.4 });
  }

  async cancelSpin() {
    await this.reels.stop(this.reels.getGrid(), { turbo: true });
  }

  async playSpin(result: SpinResult, opts: PlayOptions) {
    await presentCascades(this.reels, result.outcome, opts, {
      formatAmount: coins,
      beforeWins: async (step) => {
        const m = step.multiplier ?? 1;
        if (m > 1) {
          this.setMeter(m);
          if (!this.ctx.reducedMotion) {
            this.meter.scale.set(1.6);
            await this.t.to(this.meter.scale, { x: 1, y: 1 }, 300, ease.outBack);
          }
        }
      },
      onClear: () => this.ctx.sound.play("eyeofra/crumble", { volume: 0.6 }),
    });
    for (const e of result.outcome.events ?? []) {
      if (e.type === "max_win") await this.fx.banner("MAX WIN", { sub: `${e.value}× bet reached`, color: 0xff4fd8 });
    }
  }

  async celebrate(win: number, _bet: number, tier: WinTier, opts: PlayOptions) {
    await this.fx.celebrate(win, tier, coins, opts);
  }

  // ---------- Tomb Explorer ----------

  private enterTomb(on: boolean) {
    this.inTomb = on;
    this.board.visible = !on;
    this.tomb.visible = on;
    this.drawScenery();
    this.ctx.sound.playMusic(on ? "eyeofra/music-bonus" : "eyeofra/music-base");
  }

  private layoutTomb() {
    const { width: w, height: h } = this;
    const unit = Math.min(w, h);
    this.tombTitle.style.fontSize = Math.max(22, unit * 0.06);
    this.tombSub.style.fontSize = Math.max(14, unit * 0.032);
    this.tombTitle.x = this.tombSub.x = w / 2;
    this.tombTitle.y = h * 0.04;
    this.tombSub.y = this.tombTitle.y + this.tombTitle.height + 6;
    const n = this.urnSprites.length;
    if (n === 0) return;
    const cols = Math.ceil(Math.sqrt(n));
    const rows = Math.ceil(n / cols);
    const top = this.tombSub.y + this.tombSub.height + h * 0.03;
    const size = Math.min((w * 0.9) / cols, (h - top - h * 0.03) / rows);
    const x0 = (w - cols * size) / 2;
    this.urnSprites.forEach((s, i) => {
      s.x = x0 + (i % cols) * size + size / 2;
      s.y = top + Math.floor(i / cols) * size + size / 2;
      const k = (size * 0.9) / Math.max(s.texture.width, s.texture.height);
      s.scale.set(k);
      const label = this.urnLabels[i]!;
      label.x = s.x;
      label.y = s.y + size * 0.3;
      label.style.fontSize = Math.max(14, size * 0.15);
    });
  }

  private textureFor(u: PublicUrn) {
    return this.textures.get(u.kind === "hidden" ? "urn" : u.kind)!;
  }

  /** Builds the urns of the current chamber from public state. */
  private buildChamber(data: TombData, showAll = false) {
    this.urnLayer.removeChildren().forEach((c) => c.destroy());
    this.urnSprites = [];
    this.urnLabels = [];
    const c = data.chamber;
    const urns = data.urns[Math.min(c, data.urns.length - 1)] ?? [];
    urns.forEach((u, i) => {
      const s = new Sprite(this.textureFor(u));
      s.anchor.set(0.5);
      const hidden = u.kind === "hidden";
      s.alpha = !hidden && showAll && !u.revealed ? 0.45 : 1;
      if (hidden) {
        s.eventMode = "static";
        s.cursor = "pointer";
        s.on("pointertap", () => this.tap(i));
        s.on("pointerover", () => (s.tint = 0xfff0c0));
        s.on("pointerout", () => (s.tint = 0xffffff));
      }
      const label = new Text({
        text: u.value ? coins(u.value) : "",
        style: { fontFamily: "Cinzel, Georgia, serif", fontWeight: "700", fill: 0xffffff, stroke: { color: 0x000000, width: 5 } },
      });
      label.anchor.set(0.5);
      this.urnLayer.addChild(s, label);
      this.urnSprites.push(s);
      this.urnLabels.push(label);
    });
    const m = data.multipliers[c] ?? 1;
    this.tombTitle.text = `CHAMBER ${Math.min(c + 1, data.chambers)} OF ${data.chambers}  ·  ×${m}`;
    this.tombSub.text = `Treasure ${coins(this.collected)}  ·  ${c === 0 ? "find the passage" : "beware the curse"}  ·  tap an urn or press 1–${urns.length}`;
    this.layoutTomb();
  }

  private tap(i: number) {
    if (!this.pickResolve || !this.pickChoices.has(String(i))) return;
    const resolve = this.pickResolve;
    this.pickResolve = null;
    if (this.autoTimer) clearTimeout(this.autoTimer);
    this.ctx.sound.play("click");
    resolve({ action: "pick", choice: String(i) });
  }

  chooseBonusAction(bonus: BonusState, opts: { auto: boolean }): Promise<BonusAction> {
    const choices = bonus.actions[0]?.choices ?? [];
    this.pickChoices = new Set(choices);
    return new Promise((resolve) => {
      this.pickResolve = resolve;
      if (opts.auto && choices.length > 0) {
        this.autoTimer = setTimeout(() => this.tap(Number(choices[Math.floor(Math.random() * choices.length)])), 700);
      }
    });
  }

  async playBonusIntro(trigger: BonusTrigger, bonus: BonusState, opts: PlayOptions) {
    this.ctx.sound.play("bonus-trigger");
    await this.reels.highlightWins([{ symbol: "eye", amount: 0, positions: trigger.positions }], { turbo: opts.turbo, formatAmount: () => "" });
    this.reels.clearHighlight();
    const extra = Number((trigger.data as { bonusMultiplier?: number } | undefined)?.bonusMultiplier ?? 0);
    await this.fx.banner("TOMB EXPLORER", {
      sub: extra > 0 ? `Every chamber +${extra}×  ·  find the passage, beware the curse` : "Find the passage. Beware the curse.",
      ms: opts.turbo ? 1200 : 2200,
    });
    this.collected = 0;
    this.resumeBonus(bonus);
  }

  resumeBonus(bonus: BonusState) {
    this.tombData = bonus.data as unknown as TombData;
    this.collected = bonus.totalWin;
    this.enterTomb(true);
    this.buildChamber(this.tombData);
  }

  async playBonusStep(step: BonusStepResult, opts: PlayOptions) {
    const r = (step.reveal ?? {}) as { urn?: number; kind?: string; win?: number; nextChamber?: number; ending?: string };
    const data = step.state.data as unknown as TombData;
    const sprite = this.urnSprites[r.urn ?? -1];
    if (sprite) {
      sprite.eventMode = "none";
      sprite.tint = 0xffffff;
      this.ctx.sound.play("eyeofra/urn-open");
      if (!this.ctx.reducedMotion) {
        const x0 = sprite.x;
        await this.t.run(opts.turbo ? 200 : 380, (p) => (sprite.x = x0 + Math.sin(p * Math.PI * 8) * 8 * (1 - p)));
        sprite.x = x0;
      }
      sprite.texture = this.textures.get(r.kind ?? "urn")!;
      const k = sprite.scale.x;
      sprite.scale.set(k * 1.25);
      void this.t.to(sprite.scale, { x: k, y: k }, 300, ease.outBack);
      if (r.win) this.urnLabels[r.urn!]!.text = coins(r.win);
    }
    this.collected = step.state.totalWin;
    switch (r.kind) {
      case "treasure":
      case "pharaoh":
        this.ctx.sound.play("eyeofra/treasure");
        if (sprite) this.fx.coinBurst(sprite.getGlobalPosition().x, sprite.getGlobalPosition().y, r.kind === "pharaoh" ? 30 : 10);
        if (r.kind === "pharaoh") await this.fx.banner("PHARAOH'S TREASURE", { sub: coins(r.win ?? 0), ms: 1400 });
        break;
      case "passage":
        this.ctx.sound.play("eyeofra/passage");
        await this.t.wait(opts.turbo ? 300 : 700);
        await this.fx.banner(`CHAMBER ${(r.nextChamber ?? 0) + 1}`, { sub: `Treasure here pays ×${data.multipliers[r.nextChamber ?? 0] ?? 1}`, ms: opts.turbo ? 800 : 1400 });
        break;
      case "curse":
        this.ctx.sound.play("eyeofra/curse");
        void this.fx.shake(14, 500);
        await this.fx.flash(0x8a0000, 400);
        break;
    }
    this.tombData = data;
    if (step.state.status === "completed") {
      // Show what every urn of this chamber held.
      await this.t.wait(opts.turbo ? 300 : 700);
      this.buildChamber({ ...data, chamber: Math.min(data.chamber, data.urns.length - 1) }, true);
      await this.t.wait(opts.turbo ? 800 : 1800);
    } else if (r.kind === "passage") {
      this.buildChamber(data);
    } else {
      this.tombSub.text = `Treasure ${coins(this.collected)}  ·  ${data.chamber === 0 ? "find the passage" : "beware the curse"}`;
    }
  }

  async playBonusEnd(bonus: BonusState, opts: PlayOptions) {
    const ending = (bonus.data as unknown as TombData).ending;
    const title = ending === "curse" ? "CURSED!" : ending === "cleared" ? "TOMB CLEARED" : "MAX WIN";
    await this.fx.banner(title, { sub: `You keep ${coins(bonus.totalWin)} Coins`, color: ending === "curse" ? 0xff6b6b : GOLD, ms: opts.turbo ? 1200 : 2200 });
    this.enterTomb(false);
    this.tombData = null;
  }

  skip() {
    this.t.skip();
  }

  destroy() {
    if (this.autoTimer) clearTimeout(this.autoTimer);
    window.removeEventListener("keydown", this.onKey);
    this.ctx.app.ticker.remove(this.animate, this);
    this.ctx.sound.stopMusic();
    this.fx.destroy();
    this.t.destroy();
    this.reels.destroy();
    this.world.destroy({ children: true });
    this.overlay.destroy({ children: true });
  }
}
