import type { GameModule } from "../types";
import { ValhallaRenderer } from "./ValhallaRenderer";

const GODS: Record<string, string> = {
  odin: "Odin's ravens bring wilds",
  thor: "Thor's lightning multiplies wins",
  loki: "Loki transforms the reels",
};

const game: GameModule = {
  createRenderer: () => new ValhallaRenderer(),
  describeBonus: (bonus) => GODS[String((bonus.data as { god?: string }).god)] ?? "Ragnarök Free Spins",
};

export default game;
