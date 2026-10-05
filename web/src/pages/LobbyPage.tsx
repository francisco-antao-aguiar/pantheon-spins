import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { api, unwrap, type GameInfo, type Schemas } from "../api/client";
import { Coins, ErrorNote } from "../components/ui";
import { clientGames } from "../games/registry";
import { useSession } from "../stores/session";

type Volatility = Schemas["Volatility"];
type Layout = Schemas["LayoutKind"];

const VOLATILITY: Record<Volatility, { label: string; bars: number }> = {
  low: { label: "Low", bars: 1 },
  medium: { label: "Medium", bars: 2 },
  high: { label: "High", bars: 3 },
  very_high: { label: "Very high", bars: 4 },
};

const LAYOUT: Record<Layout, string> = {
  ways: "Ways",
  lines: "Lines",
  cluster: "Cluster pays",
  megaways: "Megaways",
  hex: "Hexagonal",
};

function VolatilityMeter({ v }: { v: Volatility }) {
  const { label, bars } = VOLATILITY[v];
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-slate-300" title={`${label} volatility`}>
      <span className="flex items-end gap-0.5" aria-hidden>
        {[1, 2, 3, 4].map((i) => (
          <span key={i} className={`w-1 rounded-sm ${i <= bars ? "bg-gold-400" : "bg-night-600"}`} style={{ height: 4 + i * 3 }} />
        ))}
      </span>
      {label}
    </span>
  );
}

function GameCard({ game }: { game: GameInfo }) {
  const client = clientGames[game.id];
  return (
    <li>
      <Link
        to={`/play/${game.id}`}
        className="group block overflow-hidden rounded-2xl border border-night-700 bg-night-900 transition hover:-translate-y-0.5 hover:border-gold-500/60 hover:shadow-xl hover:shadow-gold-600/10 focus-visible:-translate-y-0.5"
      >
        <div className="relative aspect-[5/3] overflow-hidden bg-night-800">
          {client ? (
            <img src={client.cover} alt="" className="size-full object-cover transition duration-300 group-hover:scale-105" />
          ) : (
            <div className="grid size-full place-items-center font-display text-slate-500">{game.name}</div>
          )}
        </div>
        <div className="flex flex-col gap-1 p-3">
          <div className="flex items-center justify-between gap-2">
            <h3 className="truncate font-display font-semibold">{game.name}</h3>
            <div className="flex shrink-0 gap-1">
              {game.tags.map((t) => (
                <span
                  key={t}
                  className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold tracking-wide ${t === "hot" ? "bg-rose-500 text-white" : "bg-emerald-400 text-night-950"}`}
                >
                  {t.toUpperCase()}
                </span>
              ))}
            </div>
          </div>
          <div className="flex items-center justify-between gap-2">
            <VolatilityMeter v={game.volatility} />
            <span className="text-xs text-slate-500">
              {LAYOUT[game.layout]} · {game.reels}×{game.rows}
            </span>
          </div>
        </div>
      </Link>
    </li>
  );
}

export function LobbyPage() {
  const me = useSession((s) => s.me);
  const [games, setGames] = useState<GameInfo[] | null>(null);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [volatility, setVolatility] = useState<Volatility | "">("");
  const [layout, setLayout] = useState<Layout | "">("");
  const [tag, setTag] = useState<"" | "new" | "hot">("");

  useEffect(() => {
    api.GET("/games").then(
      (r) => setGames(unwrap(r)),
      () => setError("Could not load games."),
    );
  }, []);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (games ?? []).filter(
      (g) =>
        (!q || g.name.toLowerCase().includes(q) || g.theme.toLowerCase().includes(q) || (g.description ?? "").toLowerCase().includes(q)) &&
        (!volatility || g.volatility === volatility) &&
        (!layout || g.layout === layout) &&
        (!tag || g.tags.includes(tag)),
    );
  }, [games, query, volatility, layout, tag]);

  const select = "min-h-10 rounded-xl border border-night-600 bg-night-900 px-3 text-sm";
  const filtered = query || volatility || layout || tag;

  return (
    <div className="flex flex-col gap-6">
      <section className="flex flex-wrap items-end justify-between gap-4 rounded-2xl border border-night-700 bg-gradient-to-br from-night-800 to-night-900 p-5 sm:p-7">
        <div>
          <p className="text-sm text-slate-400">Welcome, {me?.username}</p>
          <h1 className="mt-1 font-display text-2xl font-bold sm:text-3xl">Choose your realm</h1>
        </div>
        <p className="text-slate-300">
          Balance <Coins value={me?.balance ?? 0} className="text-lg text-gold-300" />
        </p>
      </section>

      <div className="flex flex-wrap gap-2">
        <input
          type="search"
          placeholder="Search games"
          aria-label="Search games"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="min-h-10 min-w-0 flex-1 rounded-xl border border-night-600 bg-night-900 px-3 text-sm placeholder:text-slate-500 sm:max-w-xs"
        />
        <select aria-label="Volatility" className={select} value={volatility} onChange={(e) => setVolatility(e.target.value as Volatility | "")}>
          <option value="">Any volatility</option>
          {Object.entries(VOLATILITY).map(([k, v]) => (
            <option key={k} value={k}>
              {v.label}
            </option>
          ))}
        </select>
        <select aria-label="Layout" className={select} value={layout} onChange={(e) => setLayout(e.target.value as Layout | "")}>
          <option value="">Any layout</option>
          {Object.entries(LAYOUT).map(([k, v]) => (
            <option key={k} value={k}>
              {v}
            </option>
          ))}
        </select>
        <div className="flex gap-1" role="group" aria-label="Tags">
          {(["new", "hot"] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTag(tag === t ? "" : t)}
              aria-pressed={tag === t}
              className={`min-h-10 rounded-xl px-3 text-sm font-semibold ${tag === t ? "bg-gold-500 text-night-950" : "border border-night-600 bg-night-900 text-slate-300"}`}
            >
              {t === "new" ? "New" : "Hot"}
            </button>
          ))}
        </div>
      </div>

      <ErrorNote>{error}</ErrorNote>
      {games === null && !error && <p className="text-slate-400">Loading games…</p>}
      {games && shown.length === 0 && (
        <div className="rounded-2xl border border-dashed border-night-600 p-10 text-center text-slate-400">
          {filtered ? "No games match these filters." : "The pantheon is still under construction."}
        </div>
      )}
      {shown.length > 0 && (
        <ul className="grid grid-cols-1 gap-4 min-[420px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {shown.map((g) => (
            <GameCard key={g.id} game={g} />
          ))}
        </ul>
      )}
    </div>
  );
}
