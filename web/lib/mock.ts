// Dev-only stand-in for the Go server (NEXT_PUBLIC_MOCK=1). Never bundled
// into the export: lib/api.ts imports it behind a build-time constant.
//
// Answers from web/fixtures. Every third poll it appends one made-up call,
// so the live pulse and the new-row tint can be seen. `?mock=empty` in the
// page URL serves the empty state.
import stats24h from "@/fixtures/stats-24h.json";
import stats7d from "@/fixtures/stats-7d.json";
import stats30d from "@/fixtures/stats-30d.json";
import statsAll from "@/fixtures/stats-all.json";
import statsEmpty from "@/fixtures/stats-empty.json";
import sessions from "@/fixtures/sessions.json";
import { NotFoundError } from "./api";
import type { RecentCall, SessionDetail, Stats } from "./types";

const fixtures: Record<string, Stats> = {
  "24h": stats24h as unknown as Stats,
  "7d": stats7d as unknown as Stats,
  "30d": stats30d as unknown as Stats,
  all: statsAll as unknown as Stats,
};

const state = new Map<string, Stats>();
let polls = 0;
let extraMs = 0;

function addCall(s: Stats) {
  const base = s.recent[0];
  if (!base) return;
  extraMs += 37_000;
  const at = new Date(Date.parse(s.generated_at) + extraMs).toISOString();
  const call: RecentCall = { ...base, at, duration_ms: 40 + (polls % 7) * 11 };
  s.recent = [call, ...s.recent].slice(0, 50);
  s.kpis = { ...s.kpis, calls: s.kpis.calls + 1 };
  s.clients = s.clients.map((c) =>
    c.id === call.client ? { ...c, calls: c.calls + 1, last_seen: at } : c,
  );
  const points = [...s.series.points];
  const last = { ...points[points.length - 1] };
  last.by_client = {
    ...last.by_client,
    [call.client]: (last.by_client[call.client] ?? 0) + 1,
  };
  points[points.length - 1] = last;
  s.series = { ...s.series, points };
}

export async function mockFetch(path: string): Promise<unknown> {
  await new Promise((r) => setTimeout(r, 120));
  const url = new URL(path, "http://mock");
  if (url.pathname === "/api/session") {
    const id = url.searchParams.get("id") ?? "";
    const s = (sessions as unknown as Record<string, SessionDetail>)[id];
    if (!s) throw new NotFoundError(path);
    return structuredClone(s);
  }
  const range = url.searchParams.get("range") ?? "7d";
  if (new URLSearchParams(window.location.search).get("mock") === "empty") {
    return { ...structuredClone(statsEmpty as unknown as Stats), range };
  }
  let s = state.get(range);
  if (!s) {
    s = structuredClone(fixtures[range] ?? fixtures["7d"]);
    state.set(range, s);
  } else if (++polls % 3 === 0) {
    for (const st of state.values()) addCall(st);
  }
  return structuredClone(s);
}
