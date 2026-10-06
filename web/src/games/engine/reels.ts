import { Container, Graphics, Sprite, Text, Texture, type Ticker } from "pixi.js";
import type { Schemas } from "../../api/client";
import type { SoundManager } from "../../audio/sound";
import { ease, type Tweener } from "./tween";

type Position = Schemas["Position"];
type Win = Schemas["Win"];

export interface ReelGridOptions {
  heights: number[];
  /** Cell size in scene units. */
  cell: number;
  textures: Map<string, Texture>;
  /** Symbols shown while reels spin. */
  blurSymbols: string[];
  tweener: Tweener;
  ticker: Ticker;
  sound: SoundManager;
  reducedMotion: boolean;
  /** Glow colour for anticipation reels and win frames. */
  accent: number;
}

interface Reel {
  box: Container; // masked
  strip: Container; // moves while spinning
  sprites: Sprite[]; // rows + 1 buffer sprite
  spinning: boolean;
  speed: number;
  glow: Graphics;
}

const SYMBOL_SCALE = 0.9;

/**
 * A grid of spinning reels. It knows nothing about game rules: it shows the
 * grids and highlights the wins it is given.
 */
export class ReelGrid extends Container {
  private reels: Reel[] = [];
  private grid: string[][] = [];
  private frames = new Container();
  private labels = new Container();
  private rng = Math.random;

  constructor(private o: ReelGridOptions) {
    super();
    const { cell, heights } = o;
    const maxRows = Math.max(...heights);
    heights.forEach((rows, r) => {
      const box = new Container();
      box.x = r * cell;
      box.y = ((maxRows - rows) * cell) / 2;
      const glow = new Graphics().roundRect(2, 2, cell - 4, rows * cell - 4, 14).fill({ color: o.accent, alpha: 0.28 });
      glow.visible = false;
      const mask = new Graphics().rect(0, 0, cell, rows * cell).fill(0xffffff);
      const strip = new Container();
      box.addChild(glow, strip, mask);
      box.mask = mask;
      const sprites: Sprite[] = [];
      for (let i = 0; i <= rows; i++) {
        const s = new Sprite(Texture.EMPTY);
        s.anchor.set(0.5);
        s.x = cell / 2;
        s.y = (i - 1) * cell + cell / 2; // sprite 0 is the buffer above the reel
        strip.addChild(s);
        sprites.push(s);
      }
      this.addChild(box);
      this.reels.push({ box, strip, sprites, spinning: false, speed: 0, glow });
    });
    this.addChild(this.frames, this.labels);
    o.ticker.add(this.tick, this);
  }

  get gridWidth() {
    return this.o.heights.length * this.o.cell;
  }

  get gridHeight() {
    return Math.max(...this.o.heights) * this.o.cell;
  }

  private texture(id: string) {
    return this.o.textures.get(id) ?? Texture.EMPTY;
  }

  private place(s: Sprite, id: string) {
    s.texture = this.texture(id);
    const size = this.o.cell * SYMBOL_SCALE;
    const k = size / Math.max(s.texture.width || 1, s.texture.height || 1);
    s.scale.set(k);
    s.alpha = 1;
    s.tint = 0xffffff;
  }

  private randomSymbol() {
    const b = this.o.blurSymbols;
    return b[Math.floor(this.rng() * b.length)]!;
  }

  /** Shows a grid immediately. */
  setGrid(grid: string[][]) {
    this.grid = grid.map((r) => [...r]);
    this.reels.forEach((reel, r) => {
      reel.spinning = false;
      reel.strip.y = 0;
      reel.sprites.forEach((s, i) => {
        s.y = (i - 1) * this.o.cell + this.o.cell / 2;
        this.place(s, i === 0 ? this.randomSymbol() : (grid[r]?.[i - 1] ?? ""));
      });
    });
  }

  getGrid() {
    return this.grid.map((r) => [...r]);
  }

  private tick(ticker: Ticker) {
    const { cell } = this.o;
    for (const reel of this.reels) {
      if (!reel.spinning) continue;
      const rows = reel.sprites.length - 1;
      const dy = reel.speed * ticker.deltaMS * this.o.tweener.speed;
      for (const s of reel.sprites) {
        s.y += dy;
        if (s.y > rows * cell + cell / 2) {
          s.y -= (rows + 1) * cell;
          this.place(s, this.randomSymbol());
          s.alpha = 0.85;
        }
      }
    }
  }

  /** Starts every reel spinning (before the result is known). */
  startSpin() {
    this.clearHighlight();
    for (const reel of this.reels) {
      reel.spinning = true;
      reel.speed = this.o.reducedMotion ? 1.2 : 2.6; // cells per… px per ms
      reel.glow.visible = false;
    }
  }

  /**
   * Stops the reels on the final grid, left to right. Reels listed in
   * anticipation spin longer, glowing, before they stop.
   */
  async stop(grid: string[][], opts: { turbo: boolean; anticipation?: number[] }) {
    const t = this.o.tweener;
    const gap = opts.turbo ? 70 : 220;
    const extra = opts.turbo ? 400 : 1100;
    let antPlayed = false;
    await t.wait(opts.turbo ? 120 : 350);
    for (let r = 0; r < this.reels.length; r++) {
      const reel = this.reels[r]!;
      if (opts.anticipation?.includes(r)) {
        reel.glow.visible = true;
        if (!antPlayed) {
          this.o.sound.play("anticipation");
          antPlayed = true;
        }
        reel.speed *= 0.6;
        await t.wait(extra);
      } else if (r > 0) {
        await t.wait(gap);
      }
      this.landReel(r, grid[r] ?? []);
    }
    this.grid = grid.map((col) => [...col]);
    await t.wait(opts.turbo ? 80 : 200);
  }

  private landReel(r: number, symbols: string[]) {
    const reel = this.reels[r]!;
    const { cell } = this.o;
    reel.spinning = false;
    reel.glow.visible = false;
    reel.sprites.forEach((s, i) => {
      s.y = (i - 1) * cell + cell / 2;
      this.place(s, i === 0 ? this.randomSymbol() : (symbols[i - 1] ?? ""));
    });
    this.o.sound.play("reel-stop", { volume: 0.5 });
    if (this.o.reducedMotion) {
      reel.strip.y = 0;
      return;
    }
    reel.strip.y = -cell * 0.35;
    void this.o.tweener.to(reel.strip, { y: 0 }, 260, ease.outBack);
  }

  /** Bursts cells that are being cleared (cascade wins). */
  async explode(positions: Position[]) {
    const t = this.o.tweener;
    await Promise.all(
      positions.map((p) => {
        const s = this.sprite(p);
        if (!s) return Promise.resolve();
        if (this.o.reducedMotion) return t.to(s, { alpha: 0 }, 150);
        const k = s.scale.x;
        return t.run(260, (q) => {
          s.scale.set(k * (1 + 0.35 * Math.sin(q * Math.PI * 0.5)) * (1 - q));
          s.alpha = 1 - q;
        });
      }),
    );
  }

  /**
   * Drops the board into `next` after `removed` cells were cleared: in each
   * reel the survivors fall to their new rows and new symbols fall in from
   * above, matching how the server tumbled the grid.
   */
  async cascadeTo(next: string[][], removed: Position[]) {
    const { cell, tweener: t } = this.o;
    const moves: Promise<void>[] = [];
    this.reels.forEach((reel, r) => {
      const gone = new Set(removed.filter((p) => p.reel === r).map((p) => p.row));
      if (gone.size === 0) return;
      const rows = reel.sprites.length - 1;
      const survivors = [...Array(rows).keys()].filter((row) => !gone.has(row));
      const k = gone.size;
      for (let row = 0; row < rows; row++) {
        const s = reel.sprites[row + 1]!;
        this.place(s, next[r]?.[row] ?? "");
        const from = row >= k ? survivors[row - k]! : row - k; // new symbols start above the reel
        const to = row * cell + cell / 2;
        s.y = from * cell + cell / 2;
        const ms = this.o.reducedMotion ? 120 : 180 + 45 * (row - from);
        moves.push(t.to(s, { y: to }, ms, this.o.reducedMotion ? ease.linear : ease.outBack));
      }
    });
    await Promise.all(moves);
    this.grid = next.map((col) => [...col]);
  }

  sprite(p: Position): Sprite | undefined {
    return this.reels[p.reel]?.sprites[p.row + 1];
  }

  /** Centre of a cell in this container's coordinates. */
  cellCenter(p: Position) {
    const reel = this.reels[p.reel]!;
    return { x: reel.box.x + this.o.cell / 2, y: reel.box.y + p.row * this.o.cell + this.o.cell / 2 };
  }

  /** Changes one cell's symbol, with a pop. */
  async setSymbol(p: Position, id: string, pop = true) {
    const s = this.sprite(p);
    if (!s) return;
    if (this.grid[p.reel]) this.grid[p.reel]![p.row] = id;
    this.place(s, id);
    if (pop && !this.o.reducedMotion) {
      const k = s.scale.x;
      s.scale.set(k * 1.35);
      await this.o.tweener.to(s.scale, { x: k, y: k }, 280, ease.outBack);
    }
  }

  /**
   * Highlights wins: losing cells dim, winning cells pulse and get a frame,
   * and each win's amount floats over its cells.
   */
  async highlightWins(wins: Win[], opts: { turbo: boolean; formatAmount: (n: number) => string }) {
    this.clearHighlight();
    if (wins.length === 0) return;
    const { cell, tweener: t } = this.o;
    const winning = new Set(wins.flatMap((w) => w.positions.map((p) => `${p.reel}:${p.row}`)));
    this.reels.forEach((reel, r) =>
      reel.sprites.forEach((s, i) => {
        if (i > 0 && !winning.has(`${r}:${i - 1}`)) s.alpha = 0.3;
      }),
    );
    for (const key of winning) {
      const [reel, row] = key.split(":").map(Number) as [number, number];
      const c = this.cellCenter({ reel, row });
      this.frames.addChild(
        new Graphics().roundRect(c.x - cell / 2 + 3, c.y - cell / 2 + 3, cell - 6, cell - 6, 12).stroke({ color: this.o.accent, width: 4, alpha: 0.95 }),
      );
    }
    // One label per win, at the centre of its cells.
    for (const w of wins) {
      const pts = w.positions.map((p) => this.cellCenter(p));
      const label = new Text({
        text: opts.formatAmount(w.amount),
        style: { fontFamily: "Cinzel, Georgia, serif", fontSize: cell * 0.26, fontWeight: "700", fill: 0xffffff, stroke: { color: 0x000000, width: 5 } },
      });
      label.anchor.set(0.5);
      label.x = pts.reduce((a, p) => a + p.x, 0) / pts.length;
      label.y = pts.reduce((a, p) => a + p.y, 0) / pts.length;
      this.labels.addChild(label);
    }
    this.o.sound.play("win-small");
    if (this.o.reducedMotion) {
      await t.wait(opts.turbo ? 300 : 900);
      return;
    }
    const pulses = [...winning].map((key) => {
      const [reel, row] = key.split(":").map(Number) as [number, number];
      const s = this.sprite({ reel, row })!;
      const k = s.scale.x;
      return t.run(opts.turbo ? 350 : 900, (p) => s.scale.set(k * (1 + 0.12 * Math.sin(p * Math.PI * 2))));
    });
    await Promise.all(pulses);
  }

  clearHighlight() {
    this.frames.removeChildren().forEach((c) => c.destroy());
    this.labels.removeChildren().forEach((c) => c.destroy());
    for (const reel of this.reels) for (const s of reel.sprites) s.alpha = 1;
  }

  override destroy() {
    this.o.ticker.remove(this.tick, this);
    super.destroy({ children: true });
  }
}
