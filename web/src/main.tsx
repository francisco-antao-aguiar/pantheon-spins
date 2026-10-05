import { StrictMode, useEffect, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes } from "react-router";
import "./index.css";
import { AppLayout } from "./components/Layouts";
import { ProfilePage } from "./pages/AppPages";
import { GamePage } from "./pages/GamePage";
import { LobbyPage } from "./pages/LobbyPage";
import { ForgotPasswordPage, LoginPage, RegisterPage, ResetPasswordPage } from "./pages/AuthPages";
import { useSession } from "./stores/session";

function Splash() {
  return <div className="grid min-h-dvh place-items-center text-slate-500">Loading…</div>;
}

/**
 * Renders children for signed-in users; otherwise redirects to /login.
 * `bare` skips the app header (full-screen game view).
 */
function RequireAuth({ children, bare = false }: { children: ReactNode; bare?: boolean }) {
  const status = useSession((s) => s.status);
  if (status === "loading") return <Splash />;
  if (status === "signedOut") return <Navigate to="/login" replace />;
  return bare ? children : <AppLayout>{children}</AppLayout>;
}

/** Renders auth pages for signed-out users; signed-in users go to the lobby. */
function GuestOnly({ children }: { children: ReactNode }) {
  const status = useSession((s) => s.status);
  if (status === "loading") return <Splash />;
  if (status === "signedIn") return <Navigate to="/" replace />;
  return children;
}

function App() {
  const load = useSession((s) => s.load);
  useEffect(() => {
    load().catch(() => useSession.setState({ status: "signedOut" }));
  }, [load]);

  return (
    <Routes>
      <Route path="/login" element={<GuestOnly><LoginPage /></GuestOnly>} />
      <Route path="/register" element={<GuestOnly><RegisterPage /></GuestOnly>} />
      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      <Route path="/reset-password" element={<ResetPasswordPage />} />
      <Route path="/" element={<RequireAuth><LobbyPage /></RequireAuth>} />
      <Route path="/profile" element={<RequireAuth><ProfilePage /></RequireAuth>} />
      <Route path="/play/:gameId" element={<RequireAuth bare><GamePage /></RequireAuth>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
