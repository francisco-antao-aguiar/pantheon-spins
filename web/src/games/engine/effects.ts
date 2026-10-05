import { Container, Graphics, Sprite, Text, type Application, type Texture, type Ticker } from "pixi.js";
import type { SoundManager } from "../../audio/sound";
import type { WinTier } from "../types";
import { ease, type Tweener } from "./tween";

interface Particle {
  s: Sprite;
  vx: number;
  vy: number;
  vr: number;
}

const TIER_LABEL: Partial<Record<WinTier, string>> = { big: "BIG WIN", mega: "MEGA WIN", epic: "EPIC WIN" };
const TIER_MS: Partial<Record<WinTier, number>> = { big: 2600, mega: 3800, epic: 5200 };

export interface EffectsOptions {
  app: Application;
  tweener: Tweener;
  sound: SoundManager;
  reducedMotion: boolean;
  /** The container that shakes (usually the whole scene). */
  shakeTarget: Container;
  /** Full-screen layer for banners and celebrations, above everything. */
  overlay: Container;
  fontFamily?: string;
  accent: number;
}

/** Juicy feedback shared by every game: shake, coins, banners, celebrations. */
export class Effects {
  private particles: Particle[] = [];
  private particleLayer = new Container();
  private coin: Texture;
  private width = 1280;
  private height = 720;

  constructor(private o: EffectsOptions) {
    const g = new Graphics()
      .circle(16, 16, 15)
      .fill(0xe0a52f)
      .circle(16, 16, 10.5)
      .stroke({ color: 0xffe08a, width: 3 })
      .rect(13, 9, 6, 14)
      .fill(0xffe08a);
    this.coin = o.app.renderer.generateTexture(g);
    g.destroy();
    o.overlay.addChild(this.particleLayer);
    o.app.ticker.add(this.tick, this);
  }

  /** Sets the overlay size in scene units. */
  resize(width: number, height: number) {
    this.width = width;
    this.height = height;
  }

  private tick(ticker: Ticker) {
    const dt = Math.min(ticker.deltaMS, 50) / 16.67;
    for (let i = this.particles.length - 1; i >= 0; i--) {
      const p = this.particles[i]!;
      p.vy += 0.35 * dt;
      p.s.x += p.vx * dt;
      p.s.y += p.vy * dt;
      p.s.rotation += p.vr * dt;
      p.s.scale.x = Math.abs(Math.cos(p.s.rotation * 2)) * p.s.scale.y; // fake 3D flip
      if (p.s.y > this.height + 40) {
        p.s.destroy();
        this.particles.splice(i, 1);
      }
    }
  }

  /** Throws coins up from a point. */
  coinBurst(x: number, y: number, count: number) {
    if (this.o.reducedMotion) return;
    for (let i = 0; i < count && this.particles.length < 220; i++) {
      const s = new Sprite(this.coin);
      s.anchor.set(0.5);
      s.x = x + (Math.random() - 0.5) * 60;
      s.y = y;
      s.scale.set(0.8 + Math.random() * 0.8);
      this.particleLayer.addChild(s);
      this.particles.push({ s, vx: (Math.random() - 0.5) * 16, vy: -10 - Math.random() * 14, vr: (Math.random() - 0.5) * 0.4 });
    }
  }

  async shake(intensity = 12, ms = 450) {
    if (this.o.reducedMotion) return;
    const t = this.o.shakeTarget;
    const x0 = t.x;
    const y0 = t.y;
    await this.o.tweener.run(ms, (p) => {
      const k = intensity * (1 - p);
      t.x = x0 + (Math.random() - 0.5) * 2 * k;
      t.y = y0 + (Math.random() - 0.5) * 2 * k;
    });
    t.x = x0;
    t.y = y0;
  }

  /** Full-screen colour flash. */
  async flash(color = 0xffffff, ms = 250) {
    const g = new Graphics().rect(0, 0, this.width, this.height).fill(color);
    g.alpha = this.o.reducedMotion ? 0.25 : 0.7;
    this.o.overlay.addChild(g);
    await this.o.tweener.to(g, { alpha: 0 }, ms, ease.linear);
    g.destroy();
  }

  private bigText(text: string, size: number, color: number) {
    const t = new Text({
      text,
      style: {
        fontFamily: this.o.fontFamily ?? "Cinzel, Georgia, serif",
        fontSize: size,
        fontWeight: "700",
        fill: color,
        stroke: { color: 0x120a00, width: Math.max(4, size / 10) },
        dropShadow: { color: 0x000000, blur: 12, distance: 0, alpha: 0.8 },
        align: "center",
      },
    });
    t.anchor.set(0.5);
    return t;
  }

  /** A centred banner that holds for ms. */
  async banner(title: string, opts: { sub?: string; color?: number; ms?: number } = {}) {
    const box = new Container();
    box.x = this.width / 2;
    box.y = this.height / 2;
    const dim = new Graphics().rect(-this.width / 2, -this.height / 2, this.width, this.height).fill({ color: 0x000000, alpha: 0.45 });
    const head = this.bigText(title, Math.min(this.width, this.height) * 0.11, opts.color ?? 0xffe08a);
    box.addChild(dim, head);
    if (opts.sub) {
      const sub = this.bigText(opts.sub, Math.min(this.width, this.height) * 0.05, 0xffffff);
      sub.y = head.height * 0.75;
      box.addChild(sub);
    }
    this.o.overlay.addChild(box);
    box.alpha = 0;
    head.scale.set(this.o.reducedMotion ? 1 : 0.4);
    await Promise.all([this.o.tweener.to(box, { alpha: 1 }, 200), this.o.tweener.to(head.scale, { x: 1, y: 1 }, 450, ease.outBack)]);
    await this.o.tweener.wait(opts.ms ?? 1400);
    await this.o.tweener.to(box, { alpha: 0 }, 250);
    box.destroy({ children: true });
  }

  /** Tiered win celebration with a counting total and a coin fountain. */
  async celebrate(win: number, tier: WinTier, format: (n: number) => string, opts: { turbo: boolean }) {
    const label = TIER_LABEL[tier];
    if (!label) return;
    const ms = (TIER_MS[tier] ?? 2500) * (opts.turbo ? 0.5 : 1);
    const t = this.o.tweener;
    this.o.sound.play("win-big");

    const box = new Container();
    box.x = this.width / 2;
    box.y = this.height / 2;
    const dim = new Graphics().rect(-this.width / 2, -this.height / 2, this.width, this.height).fill({ color: 0x000000, alpha: 0.6 });
    const unit = Math.min(this.width, this.height);
    const head = this.bigText(label, unit * (tier === "epic" ? 0.15 : 0.12), tier === "big" ? 0xffe08a : tier === "mega" ? 0xff9d3c : 0xff4fd8);
    head.y = -unit * 0.08;
    const amount = this.bigText(format(0), unit * 0.1, 0xffffff);
    amount.y = unit * 0.08;
    box.addChild(dim, head, amount);
    this.o.overlay.addChild(box);
    this.o.overlay.addChild(this.particleLayer); // keep coins on top

    if (tier !== "big") void this.shake(tier === "epic" ? 18 : 12, 600);
    head.scale.set(this.o.reducedMotion ? 1 : 0.2);
    void t.to(head.scale, { x: 1, y: 1 }, 600, ease.outElastic);

    let lastCoin = 0;
    await t.run(ms, (p) => {
      amount.text = format(Math.round(win * ease.outCubic(p)));
      if (p - lastCoin > 0.04 && p < 0.9) {
        lastCoin = p;
        this.coinBurst(this.width / 2, this.height * 0.9, tier === "epic" ? 8 : 5);
        this.o.sound.play("coin", { volume: 0.25, rate: 0.9 + p * 0.4 });
      }
      if (!this.o.reducedMotion) {
        const pulse = 1 + 0.04 * Math.sin(p * Math.PI * 12);
        amount.scale.set(pulse);
      }
    });
    amount.text = format(win);
    await t.wait(opts.turbo ? 300 : 900);
    await t.to(box, { alpha: 0 }, 300);
    box.destroy({ children: true });
  }

  destroy() {
    this.o.app.ticker.remove(this.tick, this);
    for (const p of this.particles) p.s.destroy();
    this.particles = [];
    this.coin.destroy(true);
  }
}
