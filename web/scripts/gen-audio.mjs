// Generates placeholder sound effects and music loops as 16-bit mono WAV files.
// They are synthesized so the repo needs no licensed audio; swap in real files
// with the same names later. Run: node scripts/gen-audio.mjs
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const RATE = 22050;
const OUT = join(import.meta.dirname, "..", "public", "audio");

/** Deterministic noise so regenerating gives identical files. */
let seed = 1234567;
const noise = () => {
  seed = (seed * 1103515245 + 12345) & 0x7fffffff;
  return (seed / 0x3fffffff) - 1;
};

const note = (n) => 440 * Math.pow(2, (n - 69) / 12); // MIDI note → Hz

function render(seconds, fn) {
  const n = Math.round(seconds * RATE);
  const buf = new Float32Array(n);
  for (let i = 0; i < n; i++) buf[i] = fn(i / RATE, i);
  return buf;
}

function mix(...tracks) {
  const n = Math.max(...tracks.map((t) => t.length));
  const out = new Float32Array(n);
  for (const t of tracks) for (let i = 0; i < t.length; i++) out[i] += t[i];
  return out;
}

/** Attack/decay envelope. */
const env = (t, a, d) => (t < a ? t / a : Math.exp(-(t - a) / d));

function tone(freq, seconds, { a = 0.005, d = 0.2, type = "sine", vol = 0.5, slide = 0 } = {}) {
  let phase = 0;
  return render(seconds, (t) => {
    const f = freq * Math.pow(2, (slide * t) / seconds);
    phase += (2 * Math.PI * f) / RATE;
    let s;
    if (type === "square") s = Math.sign(Math.sin(phase)) * 0.6;
    else if (type === "saw") s = ((phase / Math.PI) % 2) - 1;
    else if (type === "tri") s = (2 / Math.PI) * Math.asin(Math.sin(phase));
    else s = Math.sin(phase);
    return s * env(t, a, d) * vol;
  });
}

function noiseBurst(seconds, { a = 0.002, d = 0.1, vol = 0.5, lowpass = 0.2 } = {}) {
  let y = 0;
  return render(seconds, (t) => {
    y += lowpass * (noise() - y); // one-pole low-pass
    return y * env(t, a, d) * vol * 3;
  });
}

function seq(notes, step, opts) {
  const parts = notes.map((n, i) => {
    if (n == null) return new Float32Array(0);
    const t = tone(note(n), step * 2.5, opts);
    const pad = new Float32Array(Math.round(i * step * RATE));
    const out = new Float32Array(pad.length + t.length);
    out.set(t, pad.length);
    return out;
  });
  return mix(...parts);
}

function write(name, data, gain = 0.8) {
  let peak = 0;
  for (const v of data) peak = Math.max(peak, Math.abs(v));
  const scale = peak > 0 ? gain / peak : 0;
  const bytes = Buffer.alloc(44 + data.length * 2);
  bytes.write("RIFF", 0);
  bytes.writeUInt32LE(36 + data.length * 2, 4);
  bytes.write("WAVEfmt ", 8);
  bytes.writeUInt32LE(16, 16);
  bytes.writeUInt16LE(1, 20); // PCM
  bytes.writeUInt16LE(1, 22); // mono
  bytes.writeUInt32LE(RATE, 24);
  bytes.writeUInt32LE(RATE * 2, 28);
  bytes.writeUInt16LE(2, 32);
  bytes.writeUInt16LE(16, 34);
  bytes.write("data", 36);
  bytes.writeUInt32LE(data.length * 2, 40);
  for (let i = 0; i < data.length; i++) {
    bytes.writeInt16LE(Math.max(-32768, Math.min(32767, Math.round(data[i] * scale * 32767))), 44 + i * 2);
  }
  const file = join(OUT, name + ".wav");
  writeFileSync(file, bytes);
  console.log(`${name}.wav  ${(bytes.length / 1024).toFixed(0)} KiB`);
}

/** A loop of `bars` bars: drone + arpeggio + optional drums, seamless at the ends. */
function musicLoop({ root, scale, bpm, bars, drums, arpType = "tri", arpVol = 0.18 }) {
  const beat = 60 / bpm;
  const seconds = bars * 4 * beat;
  const drone = render(seconds, (t) => {
    const f = note(root - 12);
    return (Math.sin(2 * Math.PI * f * t) + 0.5 * Math.sin(2 * Math.PI * f * 1.5 * t)) * 0.12 * (0.8 + 0.2 * Math.sin((2 * Math.PI * t) / seconds));
  });
  const pattern = [];
  for (let b = 0; b < bars * 8; b++) {
    const chord = Math.floor(b / 8) % 2 === 0 ? 0 : -2;
    pattern.push(root + chord + scale[[0, 2, 4, 2, 0, 4, 5, 4][b % 8]]);
  }
  const arp = seq(pattern, beat / 2, { a: 0.01, d: 0.35, type: arpType, vol: arpVol });
  const parts = [drone, arp.subarray(0, drone.length)];
  if (drums) {
    for (let b = 0; b < bars * 4; b++) {
      const at = Math.round(b * beat * RATE);
      const kick = tone(55, 0.3, { d: 0.12, vol: 0.5, slide: -1.5 });
      const hit = b % 2 === 1 ? noiseBurst(0.2, { d: 0.06, vol: 0.25, lowpass: 0.6 }) : new Float32Array(0);
      const track = new Float32Array(drone.length);
      track.set(kick.subarray(0, Math.min(kick.length, drone.length - at)), at);
      if (hit.length) track.set(hit.subarray(0, Math.min(hit.length, drone.length - at)), at);
      parts.push(track);
    }
  }
  return mix(...parts).subarray(0, drone.length);
}

mkdirSync(join(OUT, "valhalla"), { recursive: true });

// Shared UI and reel sounds.
write("spin", mix(noiseBurst(0.45, { a: 0.08, d: 0.2, vol: 0.4, lowpass: 0.08 }), tone(220, 0.4, { a: 0.05, d: 0.2, vol: 0.15, slide: 1 })));
write("reel-stop", mix(tone(90, 0.18, { d: 0.05, vol: 0.6, slide: -0.5 }), noiseBurst(0.08, { d: 0.02, vol: 0.3, lowpass: 0.5 })));
write("anticipation", tone(note(57), 1.4, { a: 0.6, d: 1.2, type: "saw", vol: 0.25, slide: 1 }));
write("win-small", seq([72, 76, 79], 0.07, { d: 0.15, type: "tri", vol: 0.4 }));
write("win-big", seq([60, 64, 67, 72, 76, 79, 84, 79, 84], 0.11, { d: 0.3, type: "square", vol: 0.25 }));
write("coin", tone(note(88), 0.12, { d: 0.05, type: "square", vol: 0.25 }));
write("click", tone(note(84), 0.05, { d: 0.015, vol: 0.4 }));
write("bonus-trigger", seq([57, 64, 69, 72, 76], 0.16, { a: 0.02, d: 0.5, type: "saw", vol: 0.25 }));

// Halls of Valhalla.
write("valhalla/raven", mix(noiseBurst(0.35, { a: 0.01, d: 0.08, vol: 0.5, lowpass: 0.35 }), tone(700, 0.3, { d: 0.08, type: "saw", vol: 0.2, slide: -0.6 })));
write("valhalla/lightning", mix(noiseBurst(1.2, { a: 0.001, d: 0.35, vol: 0.8, lowpass: 0.9 }), tone(45, 1.2, { d: 0.5, vol: 0.6, slide: -0.4 })));
write("valhalla/loki", mix(tone(note(64), 0.8, { a: 0.05, d: 0.5, type: "tri", vol: 0.3, slide: 0.5 }), tone(note(67), 0.8, { a: 0.1, d: 0.5, type: "tri", vol: 0.25, slide: -0.5 })));
write("valhalla/horn", tone(note(45), 1.8, { a: 0.25, d: 1.2, type: "saw", vol: 0.35 }));
write("valhalla/music-base", musicLoop({ root: 57, scale: [0, 2, 3, 5, 7, 8, 10], bpm: 84, bars: 4, drums: false }), 0.5);
write("valhalla/music-free", musicLoop({ root: 57, scale: [0, 2, 3, 5, 7, 8, 10], bpm: 126, bars: 4, drums: true, arpType: "saw", arpVol: 0.14 }), 0.55);
