import type { Range, SessionDetail, Stats } from "./types";

// The page always calls the Go server's relative /api/... paths. With
// NEXT_PUBLIC_MOCK=1 (`pnpm dev:mock`), fixtures answer instead; the check is
// inlined at build time, and next.config.ts swaps @/lib/mock for a stub
// unless mocking, so the fixtures never reach the production export.

export class NotFoundError extends Error {}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  if (process.env.NEXT_PUBLIC_MOCK === "1") {
    const { mockFetch } = await import("@/lib/mock");
    return mockFetch(path) as Promise<T>;
  }
  const res = await fetch(path, { signal, cache: "no-store" });
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
