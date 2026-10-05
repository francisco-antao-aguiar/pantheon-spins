import type { ReactNode } from "react";
import { Link, NavLink, useNavigate } from "react-router";
import { useSession } from "../stores/session";
import { Avatar } from "./Avatar";
import { Coins, PlayMoneyNotice } from "./ui";

export function Logo({ className = "" }: { className?: string }) {
  return (
    <span className={`font-display font-bold tracking-wide ${className}`}>
      <span className="bg-gradient-to-b from-gold-300 to-gold-600 bg-clip-text text-transparent">Pantheon</span>{" "}
      <span className="text-slate-100">Spins</span>
    </span>
  );
}

export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center gap-8 px-4 py-10">
      <Link to="/" className="text-center text-3xl">
        <Logo />
      </Link>
      <section className="rounded-2xl border border-night-700 bg-night-900/80 p-6 shadow-2xl backdrop-blur sm:p-8">
        <h1 className="mb-6 font-display text-xl font-semibold">{title}</h1>
        {children}
      </section>
      <PlayMoneyNotice />
    </main>
  );
}

export function AppLayout({ children }: { children: ReactNode }) {
  const me = useSession((s) => s.me);
  const logout = useSession((s) => s.logout);
  const navigate = useNavigate();
  if (!me) return null;

  const navClass = ({ isActive }: { isActive: boolean }) =>
    `rounded-lg px-3 py-2 text-sm font-medium ${isActive ? "bg-night-700 text-gold-300" : "text-slate-300 hover:text-white"}`;

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="sticky top-0 z-10 border-b border-night-700/80 bg-night-950/85 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4">
          <Link to="/" className="text-lg sm:text-xl">
            <Logo />
          </Link>
          <nav className="ml-2 hidden gap-1 sm:flex">
            <NavLink to="/" end className={navClass}>
              Lobby
            </NavLink>
            <NavLink to="/profile" className={navClass}>
              Profile
            </NavLink>
          </nav>
          <div className="ml-auto flex items-center gap-3">
            <Coins value={me.balance} className="rounded-full bg-night-800 px-3 py-1.5 text-gold-300" />
            <Link to="/profile" aria-label="Profile">
              <Avatar id={me.avatar} size="sm" />
            </Link>
            <button
              className="hidden text-sm text-slate-400 hover:text-white sm:block"
              onClick={() => logout().finally(() => navigate("/login"))}
            >
              Sign out
            </button>
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">{children}</main>
      <footer className="px-4 pb-6">
        <PlayMoneyNotice />
      </footer>
    </div>
  );
}
