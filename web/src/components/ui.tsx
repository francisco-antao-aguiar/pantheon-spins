import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";
import { useId } from "react";

export function Button({
  variant = "primary",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "ghost" }) {
  const styles =
    variant === "primary"
      ? "bg-gradient-to-b from-gold-300 to-gold-600 text-night-950 shadow-lg shadow-gold-600/20 hover:brightness-110"
      : "border border-night-600 bg-night-800/60 text-slate-200 hover:bg-night-700";
  return (
    <button
      className={`inline-flex min-h-11 items-center justify-center gap-2 rounded-xl px-5 font-semibold transition disabled:cursor-not-allowed disabled:opacity-50 ${styles} ${className}`}
      {...props}
    />
  );
}

export function Field({
  label,
  hint,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: ReactNode }) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium text-slate-300">
        {label}
      </label>
      <input
        id={id}
        className="min-h-11 rounded-xl border border-night-600 bg-night-900 px-3.5 text-slate-100 placeholder:text-slate-500 focus:border-gold-500 focus:outline-none"
        {...props}
      />
      {hint && <p className="text-xs text-slate-400">{hint}</p>}
    </div>
  );
}

export function ErrorNote({ children }: { children: ReactNode }) {
  if (!children) return null;
  return (
    <p role="alert" className="rounded-lg border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-200">
      {children}
    </p>
  );
}

export function Coins({ value, className = "" }: { value: number; className?: string }) {
  return (
    <span className={`inline-flex items-center gap-1.5 font-semibold tabular-nums ${className}`}>
      <svg viewBox="0 0 20 20" className="size-4" aria-hidden>
        <circle cx="10" cy="10" r="9" fill="#e0a52f" />
        <circle cx="10" cy="10" r="6.5" fill="none" stroke="#ffe08a" strokeWidth="1.5" />
      </svg>
      {value.toLocaleString()}
      <span className="sr-only">Coins</span>
    </span>
  );
}

export function PlayMoneyNotice() {
  return (
    <p className="text-center text-xs text-slate-500">
      Play money only. Coins have no cash value, cannot be purchased, and cannot be withdrawn.
    </p>
  );
}
