import type { AvatarId } from "../api/client";

// Placeholder avatar art: a glyph on a themed ring. Swap for illustrated
// avatars later without changing the IDs (they are part of the API).
export const AVATARS: Record<AvatarId, { label: string; glyph: string; ring: string }> = {
  raven: { label: "Raven", glyph: "🐦‍⬛", ring: "from-slate-400 to-slate-700" },
  owl: { label: "Owl", glyph: "🦉", ring: "from-amber-300 to-amber-700" },
  serpent: { label: "Serpent", glyph: "🐍", ring: "from-emerald-300 to-emerald-700" },
  scarab: { label: "Scarab", glyph: "🪲", ring: "from-sky-300 to-indigo-700" },
  wolf: { label: "Wolf", glyph: "🐺", ring: "from-zinc-300 to-zinc-600" },
  phoenix: { label: "Phoenix", glyph: "🔥", ring: "from-orange-300 to-red-700" },
  kitsune: { label: "Kitsune", glyph: "🦊", ring: "from-fuchsia-300 to-purple-700" },
  spider: { label: "Spider", glyph: "🕷️", ring: "from-yellow-300 to-stone-700" },
  fox: { label: "Fox", glyph: "🍂", ring: "from-orange-200 to-orange-600" },
  comet: { label: "Comet", glyph: "☄️", ring: "from-cyan-200 to-violet-700" },
};

export const AVATAR_IDS = Object.keys(AVATARS) as AvatarId[];

export function Avatar({ id, size = "md" }: { id: AvatarId; size?: "sm" | "md" | "lg" }) {
  const a = AVATARS[id] ?? AVATARS.raven;
  const dims = { sm: "size-9 text-lg", md: "size-12 text-2xl", lg: "size-20 text-4xl" }[size];
  return (
    <span className={`inline-grid shrink-0 place-items-center rounded-full bg-gradient-to-br p-0.5 ${a.ring} ${dims}`}>
      <span className="grid size-full place-items-center rounded-full bg-night-900" role="img" aria-label={a.label}>
        {a.glyph}
      </span>
    </span>
  );
}
