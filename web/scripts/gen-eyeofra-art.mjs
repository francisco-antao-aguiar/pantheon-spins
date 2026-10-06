// Generates the placeholder vector art for Eye of Ra: 9 symbols in the shared
// tile style, the Tomb Explorer urn and its contents, and the lobby cover.
// Run: node scripts/gen-eyeofra-art.mjs
import { artWriter, FONT, tile } from "./art-kit.mjs";

const { out, write } = artWriter("eyeofra");

const sand = { bg: ["#3a2a12", "#140c03"], rim: "#e3b23c" };
const lapis = { bg: ["#16305e", "#060f24"], rim: "#e3b23c" };

const symbols = {
  lotus: tile("lotus", { ...lapis, glow: "#8fd3ff" }, `
  <path d="M128 70 Q148 110 128 170 Q108 110 128 70 Z" fill="#6ec3ff" stroke="#1d4f86" stroke-width="4"/>
  <path d="M128 170 Q88 150 78 100 Q112 112 128 170 Z M128 170 Q168 150 178 100 Q144 112 128 170 Z" fill="#3f9be8" stroke="#1d4f86" stroke-width="4"/>
  <path d="M128 172 Q70 172 52 132 Q92 134 128 172 Z M128 172 Q186 172 204 132 Q164 134 128 172 Z" fill="#2c7cc8" stroke="#1d4f86" stroke-width="4"/>
  <path d="M96 196 H160" stroke="url(#gold)" stroke-width="10" stroke-linecap="round"/>`),

  ankh: tile("ankh", { ...sand, glow: "#ffd36b" }, `
  <ellipse cx="128" cy="86" rx="28" ry="38" fill="none" stroke="url(#gold)" stroke-width="18"/>
  <path d="M78 126 H178 M128 126 V208" stroke="url(#gold)" stroke-width="20" stroke-linecap="round"/>`),

  djed: tile("djed", { bg: ["#20402f", "#08160e"], rim: "#e3b23c", glow: "#8affc0" }, `
  <rect x="104" y="60" width="48" height="146" rx="8" fill="#2f8f63" stroke="#0f3a24" stroke-width="5"/>
  <path d="M86 78 H170 M86 98 H170 M86 118 H170 M86 138 H170" stroke="url(#gold)" stroke-width="10" stroke-linecap="round"/>
  <rect x="96" y="196" width="64" height="14" rx="5" fill="url(#gold)"/>`),

  feather: tile("feather", { bg: ["#3b1b3a", "#160616"], rim: "#e3b23c", glow: "#ffb6f0" }, `
  <path d="M148 46 Q196 110 132 206 Q100 150 148 46 Z" fill="#f4eee4" stroke="#8a6d55" stroke-width="4"/>
  <path d="M148 52 Q140 130 132 206" stroke="#8a6d55" stroke-width="4" fill="none"/>
  <path d="M146 80 L168 92 M144 110 L172 122 M140 140 L166 150 M120 104 L138 116 M116 134 L136 146" stroke="#c9b9a6" stroke-width="3"/>`),

  jar: tile("jar", { ...sand, glow: "#ffcf8a" }, `
  <ellipse cx="128" cy="80" rx="34" ry="28" fill="#c9a26b" stroke="#5a3d1c" stroke-width="4"/>
  <circle cx="118" cy="76" r="5" fill="#2a1a08"/><circle cx="138" cy="76" r="5" fill="#2a1a08"/>
  <path d="M90 108 Q78 160 98 204 H158 Q178 160 166 108 Z" fill="#e9d3a8" stroke="#5a3d1c" stroke-width="5"/>
  <path d="M96 132 H160 M92 160 H164" stroke="#2c6fb3" stroke-width="7"/>
  <path d="M112 182 L144 182" stroke="#5a3d1c" stroke-width="4"/>`),

  cat: tile("cat", { bg: ["#1c1c28", "#08080e"], rim: "#e3b23c", glow: "#c9b6ff" }, `
  <path d="M96 60 L104 104 L84 196 H172 L152 104 L160 60 L140 92 H116 Z" fill="#1f1f2a" stroke="#e3b23c" stroke-width="4"/>
  <ellipse cx="116" cy="112" rx="7" ry="5" fill="#7dff9a"/><ellipse cx="140" cy="112" rx="7" ry="5" fill="#7dff9a"/>
  <path d="M106 150 Q128 162 150 150" stroke="url(#gold)" stroke-width="8" fill="none"/>
  <circle cx="128" cy="168" r="8" fill="url(#gold)"/>`),

  falcon: tile("falcon", { ...lapis, glow: "#ffd36b" }, `
  <path d="M60 130 Q90 80 128 84 Q166 80 196 130 Q160 116 140 124 L150 200 H106 L116 124 Q96 116 60 130 Z" fill="#7a5230" stroke="#2e1b08" stroke-width="4"/>
  <circle cx="128" cy="80" r="26" fill="#8c6038" stroke="#2e1b08" stroke-width="4"/>
  <path d="M140 76 L162 86 L144 90 Z" fill="url(#gold)"/>
  <circle cx="126" cy="76" r="5" fill="#000"/>
  <path d="M126 82 Q120 96 112 100" stroke="#1d4f86" stroke-width="4" fill="none"/>`),

  mask: tile("mask", { ...lapis, rim: "#ffe08a", glow: "#ffe08a" }, `
  <path d="M60 80 Q60 52 128 48 Q196 52 196 80 L184 196 Q128 214 72 196 Z" fill="#2a5aa8"/>
  <path d="M68 80 L80 196 M188 80 L176 196 M96 60 L100 200 M160 60 L156 200" stroke="url(#gold)" stroke-width="9"/>
  <path d="M92 70 Q128 58 164 70 L160 170 Q128 186 96 170 Z" fill="url(#gold)" stroke="#5a3d0c" stroke-width="3"/>
  <path d="M106 112 Q114 104 122 112 M134 112 Q142 104 150 112" stroke="#14213d" stroke-width="6" fill="none"/>
  <path d="M120 150 Q128 156 136 150" stroke="#8a5a12" stroke-width="4" fill="none"/>
  <rect x="120" y="170" width="16" height="30" rx="4" fill="url(#gold)"/>`),

  eye: tile("eye", { bg: ["#6b2a06", "#1c0700"], rim: "#ffe08a", glow: "#ffb347" }, `
  <circle cx="128" cy="78" r="30" fill="#ff8c1a" stroke="url(#gold)" stroke-width="6"/>
  <path d="M60 136 Q128 92 196 136 Q128 176 60 136 Z" fill="#fff3d6" stroke="#2a1600" stroke-width="7"/>
  <circle cx="128" cy="136" r="20" fill="#2a1600"/>
  <circle cx="122" cy="130" r="6" fill="#ffe08a"/>
  <path d="M112 156 Q106 180 92 190 M140 156 L168 192" stroke="#2a1600" stroke-width="7" fill="none" stroke-linecap="round"/>`, "RA"),
};
for (const [id, svg] of Object.entries(symbols)) write(id, svg);

// Tomb Explorer: a sealed urn and what it can hold.
write("urn", tile("urn", { bg: ["#2a1a0a", "#0d0703"], rim: "#b8873a", glow: "#ffcf8a" }, `
  <path d="M96 62 H160 L150 88 Q196 110 186 158 Q176 204 128 208 Q80 204 70 158 Q60 110 106 88 Z" fill="#a8743c" stroke="#3e2410" stroke-width="5"/>
  <path d="M80 130 H176 M78 160 H178" stroke="#2c6fb3" stroke-width="8"/>
  <path d="M110 112 L118 104 L126 112 L134 104 L142 112" stroke="url(#gold)" stroke-width="4" fill="none"/>
  <rect x="98" y="52" width="60" height="14" rx="6" fill="url(#gold)"/>`));

write("treasure", tile("treasure", { ...sand, glow: "#ffe08a" }, `
  <ellipse cx="128" cy="176" rx="70" ry="22" fill="#b07a1c"/>
  <circle cx="98" cy="150" r="24" fill="url(#gold)" stroke="#7a4c08" stroke-width="3"/>
  <circle cx="150" cy="146" r="26" fill="url(#gold)" stroke="#7a4c08" stroke-width="3"/>
  <circle cx="124" cy="118" r="26" fill="url(#gold)" stroke="#7a4c08" stroke-width="3"/>
  <path d="M124 104 V132 M150 132 V160" stroke="#7a4c08" stroke-width="4"/>
  <path d="M92 70 L100 84 M164 70 L156 84 M128 56 V72" stroke="#fff3c4" stroke-width="5" stroke-linecap="round"/>`));

write("passage", tile("passage", { bg: ["#1d2a3a", "#070b12"], rim: "#e3b23c", glow: "#9fd0ff" }, `
  <path d="M80 210 V100 Q128 48 176 100 V210 Z" fill="#05070b" stroke="url(#gold)" stroke-width="8"/>
  <path d="M100 210 L112 180 H144 L156 210 M112 180 L118 160 H138 L144 180" stroke="#3b5f8f" stroke-width="5" fill="none"/>
  <path d="M128 76 L136 92 L128 108 L120 92 Z" fill="#9fd0ff"/>`));

write("curse", tile("curse", { bg: ["#3a0b0b", "#120202"], rim: "#8a1a1a", glow: "#ff4a4a" }, `
  <path d="M128 208 Q78 200 94 150 Q104 120 128 120 Q152 120 162 150 Q178 200 128 208 Z" fill="#2e6b2a" stroke="#0e2a0c" stroke-width="5"/>
  <path d="M86 112 Q90 52 128 50 Q166 52 170 112 Q150 128 128 128 Q106 128 86 112 Z" fill="#3c8a36" stroke="#0e2a0c" stroke-width="5"/>
  <ellipse cx="112" cy="94" rx="9" ry="6" fill="#ff3b3b"/><ellipse cx="144" cy="94" rx="9" ry="6" fill="#ff3b3b"/>
  <path d="M128 128 L122 146 M128 128 L134 146" stroke="#ff3b3b" stroke-width="4" stroke-linecap="round"/>`));

write("pharaoh", tile("pharaoh", { ...lapis, rim: "#ffe08a", glow: "#ffe08a" }, `
  <rect x="78" y="40" width="100" height="176" rx="40" fill="url(#gold)" stroke="#5a3d0c" stroke-width="5"/>
  <path d="M92 70 Q128 54 164 70 V124 Q128 140 92 124 Z" fill="#2a5aa8"/>
  <path d="M104 92 Q112 86 120 92 M136 92 Q144 86 152 92" stroke="#ffe08a" stroke-width="5" fill="none"/>
  <path d="M100 150 L156 150 M106 168 H150 M112 186 H144" stroke="#2a5aa8" stroke-width="7"/>`, "PHARAOH"));

write("cover", `<svg xmlns="http://www.w3.org/2000/svg" width="500" height="300" viewBox="0 0 500 300">
  <defs>
    <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#140a2a"/><stop offset=".55" stop-color="#5a2410"/><stop offset="1" stop-color="#c8641a"/></linearGradient>
    <radialGradient id="sun" cx=".5" cy=".5" r=".5"><stop offset="0" stop-color="#fff2b0"/><stop offset=".5" stop-color="#ffb347"/><stop offset="1" stop-color="#ff7a1a" stop-opacity="0"/></radialGradient>
    <linearGradient id="gold" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff0b8"/><stop offset=".55" stop-color="#e3a937"/><stop offset="1" stop-color="#8a5a12"/></linearGradient>
  </defs>
  <rect width="500" height="300" fill="url(#sky)"/>
  <circle cx="250" cy="170" r="120" fill="url(#sun)"/>
  <path d="M60 300 L190 120 L320 300 Z" fill="#7a4a1c"/><path d="M190 120 L320 300 H250 Z" fill="#5a3412"/>
  <path d="M260 300 L380 150 L500 300 Z" fill="#8a5622"/><path d="M380 150 L500 300 H440 Z" fill="#663e16"/>
  <path d="M0 300 L70 210 L150 300 Z" fill="#6a3e16"/>
  <path d="M200 180 Q250 150 300 180 Q250 206 200 180 Z" fill="#fff3d6" stroke="#2a1600" stroke-width="5"/>
  <circle cx="250" cy="180" r="13" fill="#2a1600"/>
  <text x="250" y="70" text-anchor="middle" font-family="${FONT}" font-weight="700" font-size="58" fill="url(#gold)" stroke="#2a1600" stroke-width="2.5" letter-spacing="5">EYE OF RA</text>
</svg>
`);

console.log(`wrote Eye of Ra art to ${out}`);
