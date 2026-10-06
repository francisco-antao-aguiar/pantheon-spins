// Shared building blocks for the placeholder game art: every game's symbols
// use the same tile (rounded frame, coloured rim, soft glow) so the art reads
// as one family. Game generators import tile() and FONT from here.
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export const FONT = "Georgia, 'Times New Roman', serif";

/** The shared tile: dark gradient, coloured rim, soft inner glow. */
export function tile(id, { bg = ["#1d2440", "#0c1020"], rim = "#c8a24a", glow = "#ffffff" }, body, label) {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${bg[0]}"/><stop offset="1" stop-color="${bg[1]}"/></linearGradient>
    <linearGradient id="rim" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff3c4"/><stop offset=".45" stop-color="${rim}"/><stop offset="1" stop-color="#5a3d0c"/></linearGradient>
    <radialGradient id="glow" cx=".5" cy=".42" r=".55"><stop offset="0" stop-color="${glow}" stop-opacity=".35"/><stop offset="1" stop-color="${glow}" stop-opacity="0"/></radialGradient>
    <filter id="soft" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="5"/></filter>
    <linearGradient id="gold" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ffe9a8"/><stop offset=".5" stop-color="#e3a937"/><stop offset="1" stop-color="#8a5a12"/></linearGradient>
    <linearGradient id="steel" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#f2f6fa"/><stop offset=".5" stop-color="#9aa8b8"/><stop offset="1" stop-color="#4b5666"/></linearGradient>
  </defs>
  <rect x="8" y="8" width="240" height="240" rx="34" fill="url(#rim)"/>
  <rect x="16" y="16" width="224" height="224" rx="28" fill="url(#bg)"/>
  <rect x="16" y="16" width="224" height="224" rx="28" fill="url(#glow)"/>
  ${body}
  ${label ? `<text x="128" y="226" text-anchor="middle" font-family="${FONT}" font-weight="700" font-size="26" fill="#fff6dc" stroke="#2a1600" stroke-width="5" paint-order="stroke" letter-spacing="2">${label}</text>` : ""}
</svg>
<!-- ${id} -->
`;
}

/** Creates a game's art folder and returns a writer for name.svg files. */
export function artWriter(game) {
  const out = join(import.meta.dirname, "..", "src", "games", game, "art");
  mkdirSync(out, { recursive: true });
  return { out, write: (name, svg) => writeFileSync(join(out, `${name}.svg`), svg) };
}
