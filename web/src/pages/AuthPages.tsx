import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { api, ApiError, unwrap } from "../api/client";
import { AuthLayout } from "../components/Layouts";
import { Button, ErrorNote, Field } from "../components/ui";
import { useSession } from "../stores/session";

/** Form submit state: pending flag and a user-facing error message. */
function useSubmit() {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const run = async (fn: () => Promise<void>) => {
    setPending(true);
    setError("");
    try {
      await fn();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reach the server. Please try again.");
    } finally {
      setPending(false);
    }
  };
  return { pending, error, run };
}

const GOOGLE_ERRORS: Record<string, string> = {
  google_cancelled: "Google sign-in was cancelled.",
  google_unverified: "Your Google email address is not verified.",
  google_failed: "Google sign-in failed. Please try again.",
  google_disabled: "Google sign-in is not available.",
};

function GoogleButton() {
  const [enabled, setEnabled] = useState(false);
  useEffect(() => {
    api.GET("/auth/providers").then((r) => setEnabled(!!r.data?.google), () => {});
  }, []);
  if (!enabled) return null;
  return (
    <>
      <div className="my-5 flex items-center gap-3 text-xs text-slate-500">
        <span className="h-px flex-1 bg-night-700" /> or <span className="h-px flex-1 bg-night-700" />
      </div>
      <a
        href="/api/v1/auth/google/start"
        className="flex min-h-11 items-center justify-center gap-2 rounded-xl border border-night-600 bg-white font-semibold text-slate-800 hover:bg-slate-100"
      >
        <svg viewBox="0 0 48 48" className="size-5" aria-hidden>
          <path fill="#FFC107" d="M43.6 20.5H42V20H24v8h11.3C33.7 32.7 29.2 36 24 36c-6.6 0-12-5.4-12-12s5.4-12 12-12c3.1 0 5.8 1.2 7.9 3.1l5.7-5.7C34 6.1 29.3 4 24 4 12.9 4 4 12.9 4 24s8.9 20 20 20 20-8.9 20-20c0-1.3-.1-2.4-.4-3.5z" />
          <path fill="#FF3D00" d="m6.3 14.7 6.6 4.8C14.7 15.1 19 12 24 12c3.1 0 5.8 1.2 7.9 3.1l5.7-5.7C34 6.1 29.3 4 24 4 16.3 4 9.7 8.3 6.3 14.7z" />
          <path fill="#4CAF50" d="M24 44c5.2 0 9.9-2 13.4-5.2l-6.2-5.2c-2 1.5-4.5 2.4-7.2 2.4-5.2 0-9.6-3.3-11.3-8l-6.5 5C9.5 39.6 16.2 44 24 44z" />
          <path fill="#1976D2" d="M43.6 20.5H42V20H24v8h11.3c-.8 2.2-2.2 4.2-4.1 5.6l6.2 5.2C37 39.2 44 34 44 24c0-1.3-.1-2.4-.4-3.5z" />
        </svg>
        Continue with Google
      </a>
    </>
  );
}

export function LoginPage() {
  const login = useSession((s) => s.login);
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const { pending, error, run } = useSubmit();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    run(async () => {
      await login(email, password);
      navigate("/");
    });
  };

  return (
    <AuthLayout title="Welcome back">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <ErrorNote>{error || GOOGLE_ERRORS[params.get("error") ?? ""]}</ErrorNote>
        <Field label="Email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        <Field
          label="Password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <Link to="/forgot-password" className="-mt-1 self-end text-sm text-gold-400 hover:underline">
          Forgot password?
        </Link>
        <Button type="submit" disabled={pending}>
          {pending ? "Signing in…" : "Sign in"}
        </Button>
      </form>
      <GoogleButton />
      <p className="mt-6 text-center text-sm text-slate-400">
        New here?{" "}
        <Link to="/register" className="font-semibold text-gold-400 hover:underline">
          Create an account
        </Link>
      </p>
    </AuthLayout>
  );
}

export function RegisterPage() {
  const register = useSession((s) => s.register);
  const navigate = useNavigate();
  const { pending, error, run } = useSubmit();
  const [form, setForm] = useState({ username: "", email: "", password: "" });
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    run(async () => {
      await register(form.email, form.password, form.username);
      navigate("/");
    });
  };

  return (
    <AuthLayout title="Create your account">
      <p className="-mt-3 mb-5 text-sm text-slate-400">Start with 10,000 free Coins.</p>
      <form onSubmit={submit} className="flex flex-col gap-4">
        <ErrorNote>{error}</ErrorNote>
        <Field
          label="Username"
          autoComplete="username"
          required
          minLength={3}
          maxLength={20}
          pattern="[A-Za-z0-9_]+"
          hint="3–20 letters, digits or underscores. Shown on leaderboards."
          value={form.username}
          onChange={set("username")}
        />
        <Field label="Email" type="email" autoComplete="email" required value={form.email} onChange={set("email")} />
        <Field
          label="Password"
          type="password"
          autoComplete="new-password"
          required
          minLength={10}
          maxLength={128}
          hint="At least 10 characters."
          value={form.password}
          onChange={set("password")}
        />
        <Button type="submit" disabled={pending}>
          {pending ? "Creating account…" : "Create account"}
        </Button>
      </form>
      <GoogleButton />
      <p className="mt-6 text-center text-sm text-slate-400">
        Already playing?{" "}
        <Link to="/login" className="font-semibold text-gold-400 hover:underline">
          Sign in
        </Link>
      </p>
    </AuthLayout>
  );
}

export function ForgotPasswordPage() {
  const { pending, error, run } = useSubmit();
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    run(async () => {
      unwrap(await api.POST("/auth/password-reset/request", { body: { email } }));
      setSent(true);
    });
  };

  return (
    <AuthLayout title="Reset your password">
      {sent ? (
        <p className="text-slate-300">
          If an account exists for <strong className="text-white">{email}</strong>, we've sent a reset link. It expires in 1
          hour.
        </p>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <ErrorNote>{error}</ErrorNote>
          <Field label="Email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          <Button type="submit" disabled={pending}>
            {pending ? "Sending…" : "Send reset link"}
          </Button>
        </form>
      )}
      <p className="mt-6 text-center text-sm">
        <Link to="/login" className="text-gold-400 hover:underline">
          Back to sign in
        </Link>
      </p>
    </AuthLayout>
  );
}

export function ResetPasswordPage() {
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const { pending, error, run } = useSubmit();
  const [password, setPassword] = useState("");
  const [done, setDone] = useState(false);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    run(async () => {
      unwrap(await api.POST("/auth/password-reset/confirm", { body: { token, password } }));
      setDone(true);
    });
  };

  return (
    <AuthLayout title="Choose a new password">
      {done ? (
        <p className="text-slate-300">Your password has been changed and you've been signed out everywhere.</p>
      ) : !token ? (
        <ErrorNote>This reset link is incomplete. Request a new one.</ErrorNote>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <ErrorNote>{error}</ErrorNote>
          <Field
            label="New password"
            type="password"
            autoComplete="new-password"
            required
            minLength={10}
            maxLength={128}
            hint="At least 10 characters."
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <Button type="submit" disabled={pending}>
            {pending ? "Saving…" : "Set password"}
          </Button>
        </form>
      )}
      <p className="mt-6 text-center text-sm">
        <Link to="/login" className="text-gold-400 hover:underline">
          Back to sign in
        </Link>
      </p>
    </AuthLayout>
  );
}
