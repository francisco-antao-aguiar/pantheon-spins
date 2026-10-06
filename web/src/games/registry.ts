import type { GameModule } from "./types";

/**
 * Client-side game registry. Each entry lazily loads a game's renderer module
 * (its own chunk) and points to its lobby cover art. Adding a game means
 * adding one entry here, mirroring server/internal/games/catalog.
 */
interface ClientGame {
  load: () => Promise<GameModule>;
  cover: string;
  /** Lobby card accent colour. */
  accent: string;
}

const covers = import.meta.glob("./*/art/cover.svg", { eager: true, query: "?url", import: "default" }) as Record<string, string>;

export const clientGames: Record<string, ClientGame> = {
  "halls-of-valhalla": {
    load: () => import("./valhalla").then((m) => m.default),
    cover: covers["./valhalla/art/cover.svg"]!,
    accent: "#e3a937",
  },
  "eye-of-ra": {
    load: () => import("./eyeofra").then((m) => m.default),
    cover: covers["./eyeofra/art/cover.svg"]!,
    accent: "#ff9d3c",
  },
};
