import type { Range, SessionDetail, Stats } from "./types";

// The page always calls the Go server's relative /api/... paths. With
// NEXT_PUBLIC_MOCK=1 (`pnpm dev:mock`), fixtures answer instead; the check is
// inlined at build time, and next.config.ts swaps @/lib/mock for a stub
// unless mocking, so the fixtures never reach the production export.

export class NotFoundError extends Error {}

/** The server refused the call: the page was opened without the run's token. */
export class ForbiddenError extends Error {}

const TOKEN_KEY = "unpause-token";
let token: string | null | undefined;

/**
 * The run's token. `unpause mcp stats --web` opens /?t=<token>; the first call
 * reads it from the URL, keeps it in sessionStorage so a reload still works,
 * and takes it out of the address bar. Read once per page load.
 */
function runToken(): string | null {
  if (token !== undefined) return token;
  token = null;
  try {
    const url = new URL(window.location.href);
    const t = url.searchParams.get("t");
    if (t) {
      token = t;
      try {
        sessionStorage.setItem(TOKEN_KEY, t);
      } catch {}
      url.searchParams.delete("t");
      window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
    } else {
      token = sessionStorage.getItem(TOKEN_KEY);
    }
  } catch {}
  return token;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  if (process.env.NEXT_PUBLIC_MOCK === "1") {
    const { mockFetch } = await import("@/lib/mock");
    return mockFetch(path) as Promise<T>;
  }
  const t = runToken();
  const url = t ? `${path}${path.includes("?") ? "&" : "?"}t=${encodeURIComponent(t)}` : path;
  const res = await fetch(url, { signal, cache: "no-store" });
  if (res.status === 403) throw new ForbiddenError(path);
  if (res.status === 404) throw new NotFoundError(path);
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return res.json() as Promise<T>;
}

export function fetchStats(range: Range, signal?: AbortSignal) {
  return getJSON<Stats>(`/api/stats?range=${range}`, signal);
}

export function fetchSession(id: string, signal?: AbortSignal) {
  return getJSON<SessionDetail>(
    `/api/session?id=${encodeURIComponent(id)}`,
    signal,
  );
}
