import { create } from "zustand";
import { api, ApiError, unwrap, type ApiClient, type AvatarId, type Me } from "../api/client";

export type SessionStatus = "loading" | "signedOut" | "signedIn";

export interface SessionState {
  status: SessionStatus;
  me: Me | null;
  /** Loads the current user; signed out on 401. */
  load(): Promise<void>;
  login(email: string, password: string): Promise<void>;
  register(email: string, password: string, username: string): Promise<void>;
  logout(): Promise<void>;
  updateProfile(changes: { username?: string; avatar?: AvatarId }): Promise<void>;
  /** Applies a balance reported by the server (after a spin, bonus step…). */
  setBalance(balance: number): void;
}

export function createSessionStore(client: ApiClient) {
  return create<SessionState>()((set) => ({
    status: "loading",
    me: null,

    async load() {
      try {
        const me = unwrap(await client.GET("/me"));
        set({ status: "signedIn", me });
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          set({ status: "signedOut", me: null });
          return;
        }
        throw err;
      }
    },

    async login(email, password) {
      const me = unwrap(await client.POST("/auth/login", { body: { email, password } }));
      set({ status: "signedIn", me });
    },

    async register(email, password, username) {
      const me = unwrap(await client.POST("/auth/register", { body: { email, password, username } }));
      set({ status: "signedIn", me });
    },

    async logout() {
      try {
        unwrap(await client.POST("/auth/logout"));
      } finally {
        set({ status: "signedOut", me: null });
      }
    },

    async updateProfile(changes) {
      const me = unwrap(await client.PATCH("/me", { body: changes }));
      set({ me });
    },

    setBalance(balance) {
      set((s) => (s.me ? { me: { ...s.me, balance } } : s));
    },
  }));
}

export const useSession = createSessionStore(api);
