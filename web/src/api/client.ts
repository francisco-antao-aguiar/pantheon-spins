import createClient, { type Client, type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";

export type Schemas = components["schemas"];
export type Me = Schemas["Me"];
export type AvatarId = Schemas["AvatarId"];
export type GameInfo = Schemas["GameInfo"];
export type SpinResult = Schemas["SpinResult"];
export type BonusState = Schemas["BonusState"];
export type ApiClient = Client<paths>;

const CSRF_COOKIE = "ps_csrf";
const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

/** An error response from the API, with its machine-readable code. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

function readCookie(name: string): string | undefined {
  return document.cookie
    .split("; ")
    .find((c) => c.startsWith(name + "="))
    ?.slice(name.length + 1);
}

/**
 * Adds the CSRF header to state-changing requests. The server sets the
 * `ps_csrf` cookie on any response; if it's missing we fetch it first.
 */
function csrfMiddleware(baseUrl: string, fetchFn: typeof fetch): Middleware {
  let pending: Promise<string> | null = null;
  const token = async () => {
    const existing = readCookie(CSRF_COOKIE);
    if (existing) return existing;
    pending ??= fetchFn(`${baseUrl}/auth/csrf`, { credentials: "same-origin" })
      .then((r) => r.json() as Promise<Schemas["CsrfToken"]>)
      .then((j) => j.token)
      .finally(() => {
        pending = null;
      });
    return pending;
  };
  return {
    async onRequest({ request }) {
      if (!SAFE_METHODS.has(request.method)) {
        request.headers.set("X-CSRF-Token", await token());
      }
      return request;
    },
  };
}

export function createApiClient(opts: { baseUrl?: string; fetch?: typeof fetch } = {}): ApiClient {
  const baseUrl = opts.baseUrl ?? "/api/v1";
  const fetchFn = opts.fetch ?? globalThis.fetch.bind(globalThis);
  const client = createClient<paths>({ baseUrl, credentials: "same-origin", fetch: fetchFn });
  client.use(csrfMiddleware(baseUrl, fetchFn));
  return client;
}

/** Returns the response data, or throws an ApiError for error responses. */
export function unwrap<T>(res: { data?: T; error?: unknown; response: Response }): T {
  if (res.error !== undefined || !res.response.ok) {
    const err = res.error as Partial<Schemas["Error"]> | undefined;
    throw new ApiError(
      res.response.status,
      err?.code ?? "http_" + res.response.status,
      err?.message ?? "Something went wrong. Please try again.",
    );
  }
  return res.data as T;
}

export const api = createApiClient();
