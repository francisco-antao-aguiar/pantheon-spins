// Generates the placeholder vector art for Halls of Valhalla: one 256×256 SVG
// per symbol in a shared tile style, plus the raven effect sprite and the
// lobby cover. Replace the files (same names) or point the manifest at a
// sprite atlas to swap in final art. Run: node scripts/gen-valhalla-art.mjs
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const OUT = join(import.meta.dirname, "..", "src", "games", "valhalla", "art");
mkdirSync(OUT, { recursive: true });

const FONT = "Georgia, 'Times New Roman', serif";

/** The shared tile: dark gradient, coloured rim, soft inner glow. */
function tile(id, { bg = ["#1d2440", "#0c1020"], rim = "#c8a24a", glow = "#ffffff" }, body, label) {
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

/** A rune carved into a standing stone, glowing in its colour. */
function rune(id, color, path) {
  const body = `
  <path d="M74 214 Q60 120 92 54 Q128 30 166 52 Q198 120 184 214 Z" fill="#6f7782" stroke="#3a4048" stroke-width="5"/>
  <path d="M86 206 Q76 124 100 64 Q128 46 156 62 Q180 124 172 206 Z" fill="#59616c"/>
  <path d="${path}" fill="none" stroke="${color}" stroke-width="18" stroke-linecap="round" stroke-linejoin="round" filter="url(#soft)" opacity=".9"/>
  <path d="${path}" fill="none" stroke="#fffaf0" stroke-width="7" stroke-linecap="round" stroke-linejoin="round"/>`;
  return tile(id, { bg: ["#232a35", "#0e1118"], rim: "#8c96a3", glow: color }, body);
}

const symbols = {
  fehu: rune("fehu", "#ff6b4a", "M116 70 V190 M116 110 L156 82 M116 142 L156 114"),
  uruz: rune("uruz", "#4ab8ff", "M104 190 V74 L154 104 V190"),
  thurisaz: rune("thurisaz", "#5bea8c", "M112 70 V190 M112 100 L152 130 L112 160"),
  ansuz: rune("ansuz", "#c48bff", "M112 70 V190 M112 76 L154 104 M112 116 L154 144"),

  shield: tile("shield", { bg: ["#1c3156", "#0b1426"], glow: "#7fb4ff" }, `
  <circle cx="128" cy="124" r="86" fill="url(#steel)"/>
  <circle cx="128" cy="124" r="76" fill="#8a4b1f"/>
  <path d="M128 48 V200 M58 124 H198" stroke="#5c2f10" stroke-width="10"/>
  <path d="M72 80 L184 168 M184 80 L72 168" stroke="#a65e2a" stroke-width="5" opacity=".7"/>
  <circle cx="128" cy="124" r="26" fill="url(#gold)" stroke="#5a3d0c" stroke-width="4"/>
  <circle cx="120" cy="116" r="7" fill="#fff6d0" opacity=".8"/>`),

  axe: tile("axe", { bg: ["#3a1d1d", "#160909"], glow: "#ff8f6b" }, `
  <rect x="118" y="44" width="20" height="170" rx="8" fill="#7a4a22" stroke="#3f230c" stroke-width="4" transform="rotate(-28 128 128)"/>
  <path d="M150 52 Q214 70 206 132 Q174 112 140 118 Z" fill="url(#steel)" stroke="#39414c" stroke-width="5"/>
  <path d="M154 64 Q196 80 196 116" fill="none" stroke="#ffffff" stroke-width="4" opacity=".7"/>
  <rect x="112" y="166" width="30" height="14" rx="4" fill="url(#gold)" transform="rotate(-28 128 128)"/>`),

  horn: tile("horn", { bg: ["#3b2a12", "#140d04"], glow: "#ffd27a" }, `
  <path d="M52 92 Q120 70 196 74 Q214 120 186 176 Q168 196 146 186 Q166 148 150 118 Q104 110 64 124 Z" fill="#efe1c0" stroke="#6b5226" stroke-width="5"/>
  <path d="M52 92 L64 124" stroke="url(#gold)" stroke-width="14" stroke-linecap="round"/>
  <path d="M112 82 Q120 100 112 116 M152 78 Q160 100 152 120" stroke="url(#gold)" stroke-width="10" fill="none"/>
  <ellipse cx="58" cy="108" rx="10" ry="18" fill="#8f3a1a"/>`),

  valkyrie: tile("valkyrie", { bg: ["#2b2152", "#0f0a24"], glow: "#d6c4ff" }, `
  <path d="M30 120 Q60 70 104 96 Q76 104 70 126 Q58 116 30 120 Z M226 120 Q196 70 152 96 Q180 104 186 126 Q198 116 226 120 Z" fill="#f4f1ff" stroke="#7b6bb3" stroke-width="4"/>
  <path d="M84 150 Q84 76 128 72 Q172 76 172 150 Z" fill="url(#steel)" stroke="#3b4350" stroke-width="5"/>
  <rect x="80" y="144" width="96" height="18" rx="6" fill="url(#gold)"/>
  <path d="M128 96 V176" stroke="#3b4350" stroke-width="10"/>
  <path d="M100 178 L156 178 L144 200 L112 200 Z" fill="#3b4350"/>`),

  wild: tile("wild", { bg: ["#173a26", "#06150c"], rim: "#e3b23c", glow: "#9dff9a" }, `
  <path d="M120 200 Q118 160 124 132 L132 132 Q138 160 136 200 Z" fill="#6b4421"/>
  <path d="M120 196 Q96 200 84 214 M136 196 Q160 200 172 214 M128 198 V216" stroke="#6b4421" stroke-width="8" fill="none" stroke-linecap="round"/>
  <circle cx="128" cy="96" r="46" fill="#3fbf63"/>
  <circle cx="88" cy="114" r="32" fill="#2ea052"/>
  <circle cx="168" cy="114" r="32" fill="#2ea052"/>
  <circle cx="104" cy="78" r="26" fill="#59d97a"/>
  <circle cx="154" cy="80" r="26" fill="#59d97a"/>
  <circle cx="128" cy="96" r="70" fill="none" stroke="#c6ffb8" stroke-width="3" opacity=".5"/>`, "WILD"),

  bonus_odin: tile("bonus_odin", { bg: ["#1d3a6e", "#081428"], rim: "#e3b23c", glow: "#9fd0ff" }, `
  <path d="M54 92 Q128 52 202 92 Q128 80 54 92 Z" fill="#283a5e"/>
  <path d="M84 92 Q84 52 128 46 Q172 52 172 92 Z" fill="#33496f" stroke="#1b2740" stroke-width="4"/>
  <ellipse cx="128" cy="112" rx="40" ry="36" fill="#f0c9a0"/>
  <path d="M92 104 L164 120" stroke="#1b1b1b" stroke-width="7"/>
  <ellipse cx="146" cy="112" rx="13" ry="10" fill="#1b1b1b"/>
  <circle cx="110" cy="110" r="6" fill="#2b4f8f"/>
  <path d="M88 124 Q92 196 128 200 Q164 196 168 124 Q148 140 128 140 Q108 140 88 124 Z" fill="#e8ecf2" stroke="#9aa3b0" stroke-width="3"/>`, "ODIN"),

  bonus_thor: tile("bonus_thor", { bg: ["#6e1d1d", "#280808"], rim: "#e3b23c", glow: "#ffd36b" }, `
  <path d="M138 30 L104 104 L132 104 L112 150" fill="none" stroke="#fff27a" stroke-width="8" stroke-linejoin="round" filter="url(#soft)"/>
  <path d="M138 30 L104 104 L132 104 L112 150" fill="none" stroke="#ffffff" stroke-width="3" stroke-linejoin="round"/>
  <rect x="116" y="110" width="24" height="86" rx="6" fill="#7a4a22" stroke="#3f230c" stroke-width="4"/>
  <rect x="62" y="70" width="132" height="58" rx="10" fill="url(#steel)" stroke="#39414c" stroke-width="5"/>
  <path d="M74 86 H182 M74 112 H182" stroke="#6c7684" stroke-width="4"/>
  <circle cx="128" cy="99" r="12" fill="url(#gold)"/>`, "THOR"),

  bonus_loki: tile("bonus_loki", { bg: ["#16502e", "#05180c"], rim: "#e3b23c", glow: "#7dffb0" }, `
  <path d="M70 70 Q50 30 82 22 Q74 50 98 74 Z M186 70 Q206 30 174 22 Q182 50 158 74 Z" fill="url(#gold)" stroke="#5a3d0c" stroke-width="3"/>
  <path d="M78 80 Q128 54 178 80 L170 156 Q128 196 86 156 Z" fill="#2c7a4b" stroke="#0f3a20" stroke-width="5"/>
  <path d="M96 112 Q110 100 122 114 Q108 120 96 112 Z M160 112 Q146 100 134 114 Q148 120 160 112 Z" fill="#c8ff5a"/>
  <path d="M100 150 Q128 168 156 150 Q128 158 100 150 Z" fill="#0f3a20"/>
  <path d="M128 176 Q116 196 128 206 Q140 196 128 176 Z M104 172 Q92 190 102 200 M152 172 Q164 190 154 200" fill="#7dffb0" stroke="#7dffb0" stroke-width="3" opacity=".8"/>`, "LOKI"),
};

for (const [id, svg] of Object.entries(symbols)) {
  writeFileSync(join(OUT, `${id}.svg`), svg);
}

// Effect sprite: a raven in flight.
writeFileSync(
  join(OUT, "raven.svg"),
  `<svg xmlns="http://www.w3.org/2000/svg" width="160" height="110" viewBox="0 0 160 110">
  <path d="M8 40 Q50 0 82 44 Q112 0 152 34 Q118 40 100 62 L120 78 L96 74 L88 96 L80 74 Q60 70 52 58 Q30 44 8 40 Z" fill="#0d0d14" stroke="#3b3f5c" stroke-width="3"/>
  <circle cx="92" cy="58" r="3.5" fill="#ffd34d"/>
  <path d="M100 62 L114 60 L101 66 Z" fill="#c8a24a"/>
</svg>
`,
);

// Lobby cover (5:3).
writeFileSync(
  join(OUT, "cover.svg"),
  `<svg xmlns="http://www.w3.org/2000/svg" width="500" height="300" viewBox="0 0 500 300">
  <defs>
    <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0b1633"/><stop offset=".6" stop-color="#1d3a6e"/><stop offset="1" stop-color="#3a2a5c"/></linearGradient>
    <linearGradient id="aur" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#5bffb0" stop-opacity="0"/><stop offset=".5" stop-color="#5bffb0" stop-opacity=".55"/><stop offset="1" stop-color="#9d7bff" stop-opacity="0"/></linearGradient>
    <linearGradient id="gold" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff0b8"/><stop offset=".55" stop-color="#e3a937"/><stop offset="1" stop-color="#8a5a12"/></linearGradient>
  </defs>
  <rect width="500" height="300" fill="url(#sky)"/>
  <path d="M0 90 Q120 30 250 80 T500 60 L500 110 Q380 130 250 120 T0 140 Z" fill="url(#aur)"/>
  <path d="M0 230 L70 150 L120 200 L190 120 L260 210 L330 140 L400 200 L460 160 L500 190 L500 300 L0 300 Z" fill="#141a2e"/>
  <path d="M190 120 L210 142 L180 140 Z M330 140 L348 160 L318 158 Z" fill="#e8f0ff" opacity=".8"/>
  <path d="M160 300 L160 200 L250 150 L340 200 L340 300 Z" fill="#2a1d10" stroke="url(#gold)" stroke-width="4"/>
  <path d="M150 204 L250 146 L350 204" fill="none" stroke="url(#gold)" stroke-width="8" stroke-linejoin="round"/>
  <rect x="226" y="236" width="48" height="64" rx="24" fill="#ffcf6a" opacity=".85"/>
  <text x="250" y="58" text-anchor="middle" font-family="${FONT}" font-weight="700" font-size="40" fill="url(#gold)" stroke="#2a1600" stroke-width="2" letter-spacing="3">HALLS OF</text>
  <text x="250" y="108" text-anchor="middle" font-family="${FONT}" font-weight="700" font-size="54" fill="url(#gold)" stroke="#2a1600" stroke-width="2.5" letter-spacing="4">VALHALLA</text>
</svg>
`,
);

console.log(`wrote ${Object.keys(symbols).length} symbols, raven and cover to ${OUT}`);
