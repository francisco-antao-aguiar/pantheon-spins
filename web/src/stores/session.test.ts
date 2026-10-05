import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient, type Me } from "../api/client";
import { createSessionStore } from "./session";

const me: Me = {
  id: "00000000-0000-0000-0000-000000000001",
  email: "odin@asgard.test",
  username: "odin",
  avatar: "raven",
  balance: 10_000,
  hasPassword: true,
  createdAt: "2026-10-05T00:00:00Z",
};

function json(status: number, body?: unknown) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Route = (req: Request) => Response | undefined;

function setup(...routes: Route[]) {
  const calls: Request[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(new URL(String(input), "http://app.test"), init);
    calls.push(req);
    const path = new URL(req.url).pathname;
    if (path === "/api/v1/auth/csrf") return json(200, { token: "csrf-tok" });
    for (const r of routes) {
      const res = r(req);
      if (res) return res;
    }
    return json(404, { code: "not_found", message: "no route " + path });
  });
  const client = createApiClient({ baseUrl: "http://app.test/api/v1", fetch: fetchMock as typeof fetch });
  return { store: createSessionStore(client), calls };
}

const route =
  (method: string, path: string, res: () => Response): Route =>
  (req) =>
    req.method === method && new URL(req.url).pathname === "/api/v1" + path ? res() : undefined;

describe("session store", () => {
  beforeEach(() => {
    document.cookie = "ps_csrf=; max-age=0";
  });

  it("starts loading and becomes signed in when /me succeeds", async () => {
    const { store } = setup(route("GET", "/me", () => json(200, me)));
    expect(store.getState().status).toBe("loading");
    await store.getState().load();
    expect(store.getState()).toMatchObject({ status: "signedIn", me });
  });

  it("becomes signed out on 401", async () => {
    const { store } = setup(route("GET", "/me", () => json(401, { code: "unauthenticated", message: "Please sign in." })));
    await store.getState().load();
    expect(store.getState()).toMatchObject({ status: "signedOut", me: null });
  });

  it("rethrows unexpected errors from load", async () => {
    const { store } = setup(route("GET", "/me", () => json(500, { code: "internal", message: "boom" })));
    await expect(store.getState().load()).rejects.toBeInstanceOf(ApiError);
  });

  it("sends the CSRF token on login and stores the user", async () => {
    const { store, calls } = setup(route("POST", "/auth/login", () => json(200, me)));
    await store.getState().login("odin@asgard.test", "pw");
    const login = calls.find((c) => c.url.endsWith("/auth/login"))!;
    expect(login.headers.get("X-CSRF-Token")).toBe("csrf-tok");
    expect(store.getState().me?.username).toBe("odin");
  });

  it("uses the CSRF cookie when present instead of fetching a token", async () => {
    document.cookie = "ps_csrf=from-cookie";
    const { store, calls } = setup(route("POST", "/auth/login", () => json(200, me)));
    await store.getState().login("odin@asgard.test", "pw");
    expect(calls.some((c) => c.url.endsWith("/auth/csrf"))).toBe(false);
    expect(calls[0]!.headers.get("X-CSRF-Token")).toBe("from-cookie");
  });

  it("surfaces the server's error code and message on failed login", async () => {
    const { store } = setup(
      route("POST", "/auth/login", () => json(401, { code: "invalid_credentials", message: "Incorrect email or password." })),
    );
    const err = await store.getState().login("x@y.z", "bad").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 401, code: "invalid_credentials", message: "Incorrect email or password." });
    expect(store.getState().status).toBe("loading");
  });

  it("signs out locally even if the logout request fails", async () => {
    const { store } = setup(
      route("GET", "/me", () => json(200, me)),
      route("POST", "/auth/logout", () => json(500, { code: "internal", message: "boom" })),
    );
    await store.getState().load();
    await expect(store.getState().logout()).rejects.toBeInstanceOf(ApiError);
    expect(store.getState()).toMatchObject({ status: "signedOut", me: null });
  });

  it("updates the balance only when signed in", async () => {
    const { store } = setup(route("GET", "/me", () => json(200, me)));
    store.getState().setBalance(5);
    expect(store.getState().me).toBeNull();
    await store.getState().load();
    store.getState().setBalance(12_345);
    expect(store.getState().me?.balance).toBe(12_345);
  });

  it("applies profile updates", async () => {
    const { store } = setup(
      route("GET", "/me", () => json(200, me)),
      route("PATCH", "/me", () => json(200, { ...me, avatar: "wolf" })),
    );
    await store.getState().load();
    await store.getState().updateProfile({ avatar: "wolf" });
    expect(store.getState().me?.avatar).toBe("wolf");
  });
});
