import type { GameModule } from "../types";
import { EyeOfRaRenderer } from "./EyeOfRaRenderer";

const game: GameModule = {
  createRenderer: () => new EyeOfRaRenderer(),
  describeBonus: (bonus) => {
    const d = bonus.data as { chamber?: number; chambers?: number };
    return `Tomb Explorer · chamber ${Math.min((d.chamber ?? 0) + 1, d.chambers ?? 3)} of ${d.chambers ?? 3} · tap an urn or press 1–9`;
  },
};

export default game;
