import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { ApiError, type AvatarId } from "../api/client";
import { Avatar, AVATAR_IDS, AVATARS } from "../components/Avatar";
import { Button, ErrorNote, Field } from "../components/ui";
import { useSession } from "../stores/session";

export function ProfilePage() {
  const me = useSession((s) => s.me)!;
  const updateProfile = useSession((s) => s.updateProfile);
  const logout = useSession((s) => s.logout);
  const navigate = useNavigate();
  const [username, setUsername] = useState(me.username);
  const [avatar, setAvatar] = useState<AvatarId>(me.avatar);
  const [status, setStatus] = useState<{ pending: boolean; error: string; saved: boolean }>({
    pending: false,
    error: "",
    saved: false,
  });

  const dirty = username !== me.username || avatar !== me.avatar;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setStatus({ pending: true, error: "", saved: false });
    try {
      await updateProfile({
        ...(username !== me.username && { username }),
        ...(avatar !== me.avatar && { avatar }),
      });
      setStatus({ pending: false, error: "", saved: true });
    } catch (err) {
      setStatus({ pending: false, error: err instanceof ApiError ? err.message : "Could not save.", saved: false });
    }
  };

  return (
    <div className="mx-auto flex max-w-xl flex-col gap-6">
      <div className="flex items-center gap-4">
        <Avatar id={avatar} size="lg" />
        <div>
          <h1 className="font-display text-2xl font-bold">{me.username}</h1>
          <p className="text-sm text-slate-400">{me.email}</p>
          <p className="text-xs text-slate-500">Playing since {new Date(me.createdAt).toLocaleDateString()}</p>
        </div>
      </div>

      <form onSubmit={submit} className="flex flex-col gap-5 rounded-2xl border border-night-700 bg-night-900 p-6">
        <ErrorNote>{status.error}</ErrorNote>
        <Field
          label="Username"
          required
          minLength={3}
          maxLength={20}
          pattern="[A-Za-z0-9_]+"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <fieldset>
          <legend className="mb-2 text-sm font-medium text-slate-300">Avatar</legend>
          <div className="grid grid-cols-5 gap-3">
            {AVATAR_IDS.map((id) => (
              <button
                type="button"
                key={id}
                onClick={() => setAvatar(id)}
                aria-pressed={avatar === id}
                title={AVATARS[id].label}
                className={`grid place-items-center rounded-xl p-1.5 transition ${avatar === id ? "bg-night-700 ring-2 ring-gold-400" : "hover:bg-night-800"}`}
              >
                <Avatar id={id} />
              </button>
            ))}
          </div>
        </fieldset>
        <div className="flex items-center gap-3">
          <Button type="submit" disabled={!dirty || status.pending}>
            {status.pending ? "Saving…" : "Save changes"}
          </Button>
          {status.saved && !dirty && <span className="text-sm text-emerald-400">Saved</span>}
        </div>
      </form>

      <Button variant="ghost" onClick={() => logout().finally(() => navigate("/login"))}>
        Sign out
      </Button>
    </div>
  );
}
