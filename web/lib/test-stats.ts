// A small Stats to build test cases from (vitest only).
import type { ClientStat, Stats } from "./types";

export function client(id: string, name: string, slot: number, calls: number, sessions_read = 0): ClientStat {
  return { id, name, slot, calls, sessions_read, last_seen: "2026-10-15T13:00:00Z" };
}

export function stats(over: Partial<Stats> = {}): Stats {
  const zero = { calls: 0, sessions_read: 0, clients: 0, errors: 0, p50_ms: null };
  return {
    range: "7d",
    generated_at: "2026-10-15T13:40:00Z",
    empty: false,
    kpis: { ...zero, prev: zero },
    clients: [],
    series: { bucket: "day", points: [] },
    most_read: [],
    top_queries: [],
    tools: [],
    recent: [],
    ...over,
  };
}
