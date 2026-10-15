// Writes web/fixtures/*.json: sample /api/stats and /api/session responses
// for `pnpm dev` without the Go server. Deterministic (seeded PRNG, fixed
// "now"), so rerunning it gives the same files. All names are made up.
//
//   node scripts/gen-fixtures.mjs
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const out = join(dirname(fileURLToPath(import.meta.url)), "..", "fixtures");
mkdirSync(out, { recursive: true });

const NOW = Date.parse("2026-10-15T15:40:00Z");
const HOUR = 3600e3;
const DAY = 24 * HOUR;

let seed = 42;
const rand = () => {
  seed = (seed * 1664525 + 1013904223) % 4294967296;
  return seed / 4294967296;
};
const pick = (arr) => arr[Math.floor(rand() * arr.length)];
const weighted = (pairs) => {
  const sum = pairs.reduce((s, [, w]) => s + w, 0);
  let r = rand() * sum;
  for (const [v, w] of pairs) if ((r -= w) <= 0) return v;
  return pairs[pairs.length - 1][0];
};
const hex = (n) =>
  Array.from({ length: n }, () => Math.floor(rand() * 16).toString(16)).join("");
const uuid = () => `${hex(8)}-${hex(4)}-4${hex(3)}-a${hex(3)}-${hex(12)}`;

const friendly = (id) => {
  if (id.startsWith("local-agent-mode-")) return "Claude Cowork";
  if (id === "claude-ai") return "Claude Desktop";
  if (id === "openai-mcp" || id.toLowerCase().includes("chatgpt")) return "ChatGPT";
  if (id.startsWith("codex")) return "Codex";
  return id;
};

const sessions = [
  ["Refactor the billing webhook retries", "work", "billing-api"],
  ["Why does the CSV import drop the last row", "work", "importer"],
  ["Plan the onboarding email sequence", "personal", "newsletter"],
  ["Fix flaky auth test on CI", "work", "billing-api"],
  ["Sketch a tide-clock watch face", "personal", "watchfaces"],
  ["Migrate the blog to Astro", "personal", "blog"],
  ["Tune the Postgres connection pool", "work", "platform"],
  ["Draft the Q4 roadmap one-pager", "cowork", "planning"],
  ["Compare three static site hosts", "cowork", "research"],
  ["Rust port of the markdown linter", "personal", "mdlint"],
  ["Grocery list optimizer side project", "personal", "pantry"],
  ["Upgrade the design tokens to v2", "work", "design-system"],
  ["Debug the iOS share sheet crash", "work", "mobile"],
  ["Write release notes for 2.3", "work", "platform"],
].map(([title, account, project]) => ({ id: uuid(), title, account, project }));
// One Session that is no longer in the index: the page shows its short id.
sessions.push({ id: uuid(), title: "", account: "", project: "" });

const queries = [
  "webhook retry",
  "csv import",
  "\"connection pool\"",
  "astro migration",
  "flaky test",
  "roadmap",
  "design tokens",
  "share sheet crash",
  "fxtwitter",
  "release notes",
];

// Clients in order of first appearance (that order is their color slot).
const clients = [
  { id: "local-agent-mode-unpause", from: 4.3 * DAY, weight: 6 },
  { id: "openai-mcp", from: 3.9 * DAY, weight: 4 },
  { id: "codex-mcp-client", from: 2.6 * DAY, weight: 3 },
  { id: "claude-ai", from: 1.8 * DAY, weight: 2 },
  { id: "cursor-vscode", from: 1.2 * DAY, weight: 1 },
  { id: "zed", from: 0.9 * DAY, weight: 0.8 },
];

// Generate the log: activity comes in bursts, busier during the day.
const entries = [];
let t = NOW - 4.3 * DAY;
while (t < NOW - 2 * 60e3) {
  const hourOfDay = new Date(t).getUTCHours();
  const busy = hourOfDay >= 7 && hourOfDay <= 21 ? 1 : 0.15;
  const recentBoost = t > NOW - DAY ? 1.6 : 1;
  t += (-Math.log(1 - rand()) * 24 * 60e3) / (busy * recentBoost);
  if (t >= NOW) break;
  const live = clients.filter((c) => NOW - c.from <= t);
  const client = weighted(live.map((c) => [c, c.weight]));
  const burst = 1 + Math.floor(rand() * 4);
  for (let b = 0; b < burst; b++) {
    const at = t + b * (5e3 + rand() * 40e3);
    if (at >= NOW) break;
    const tool = weighted([
      ["search_sessions", 4],
      ["get_session", 5],
      ["list_sessions", 1.5],
    ]);
    const e = {
      at: new Date(Math.round(at / 1000) * 1000).toISOString(),
      client: client.id,
      tool,
      input: {},
      session_ids: [],
      count: 0,
      total: 0,
      duration_ms: Math.round(18 + Math.exp(rand() * 4.2) * 3),
      error: "",
    };
    if (tool === "search_sessions") {
      e.input = { query: pick(queries), limit: 10 };
      const n = Math.floor(rand() * 5);
      e.session_ids = Array.from({ length: n }, () => pick(sessions).id);
      e.count = n;
      e.total = n;
    } else if (tool === "get_session") {
      const s = weighted(sessions.map((s, i) => [s, 1 / (i + 1)]));
      const offset = rand() < 0.3 ? 120 : 0;
      e.input = offset ? { id: s.id, offset } : { id: s.id };
      e.session_ids = [s.id];
      e.total = 80 + Math.floor(rand() * 600);
      e.count = Math.min(e.total - offset, 40 + Math.floor(rand() * 160));
      e.duration_ms += 40;
    } else {
      e.input = rand() < 0.5 ? { limit: 20 } : { account: "work", limit: 20 };
      e.total = 214;
      e.count = 20;
      e.session_ids = Array.from({ length: 20 }, () => pick(sessions).id);
    }
    if (rand() < 0.02) {
      e.error = pick(["not found", "invalid input", "canceled", "failed"]);
      e.session_ids = [];
      e.count = 0;
      e.total = 0;
    }
    entries.push(e);
  }
}
entries.sort((a, b) => a.at.localeCompare(b.at));

const slotOf = new Map();
for (const e of entries) if (!slotOf.has(e.client)) slotOf.set(e.client, slotOf.size);
const firstAt = Date.parse(entries[0].at);

const p50 = (xs) => {
  if (!xs.length) return null;
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.floor((s.length - 1) / 2)];
};
const kpisOf = (list) => {
  const ok = list.filter((e) => !e.error);
  const read = new Set(
    ok.filter((e) => e.tool === "get_session").flatMap((e) => e.session_ids),
  );
  return {
    calls: list.length,
    sessions_read: read.size,
    clients: new Set(list.map((e) => e.client)).size,
    errors: list.length - ok.length,
    p50_ms: p50(ok.map((e) => e.duration_ms)),
  };
};
const tally = (keys, n) => {
  const m = new Map();
  for (const k of keys) m.set(k, (m.get(k) || 0) + 1);
  return [...m.entries()]
    .sort((a, b) => b[1] - a[1] || String(a[0]).localeCompare(String(b[0])))
    .slice(0, n);
};
const startOfUTCDay = (ms) => ms - (ms % DAY);

function stats(range) {
  const span = { "24h": DAY, "7d": 7 * DAY, "30d": 30 * DAY }[range];
  const from = span ? NOW - span : firstAt;
  const inRange = entries.filter((e) => Date.parse(e.at) >= from);
  const prevList = span
    ? entries.filter((e) => {
        const at = Date.parse(e.at);
        return at >= from - span && at < from;
      })
    : null;

  const bucket = range === "24h" ? "hour" : "day";
  const size = bucket === "hour" ? HOUR : DAY;
  let starts = [];
  if (bucket === "hour") {
    const last = NOW - (NOW % HOUR);
    for (let i = 24; i >= 0; i--) starts.push(last - i * HOUR);
  } else {
    const last = startOfUTCDay(NOW);
    const n = range === "7d" ? 8 : range === "30d" ? 31 : null;
    if (n) for (let i = n - 1; i >= 0; i--) starts.push(last - i * DAY);
    else for (let d = startOfUTCDay(firstAt); d <= last; d += DAY) starts.push(d);
  }
  const points = starts.map((s) => {
    const covered = s + size > firstAt;
    const by = {};
    if (covered) {
      for (const e of inRange) {
        const at = Date.parse(e.at);
        if (at >= s && at < s + size) by[e.client] = (by[e.client] || 0) + 1;
      }
    }
    return { t: new Date(s).toISOString(), covered, by_client: by };
  });

  const clientList = tally(
    inRange.map((e) => e.client),
    99,
  ).map(([id, calls]) => ({
    id,
    name: friendly(id),
    slot: slotOf.get(id) < 5 ? slotOf.get(id) : -1,
    calls,
    last_seen: inRange.filter((e) => e.client === id).at(-1).at,
  }));

  const reads = inRange.filter((e) => e.tool === "get_session" && !e.error);
  const mostRead = tally(
    reads.map((e) => e.session_ids[0]),
    10,
  ).map(([id, n]) => {
    const s = sessions.find((x) => x.id === id);
    return {
      id,
      title: s.title || id.slice(0, 8),
      account: s.account,
      project: s.project,
      reads: n,
      last_read: reads.filter((e) => e.session_ids[0] === id).at(-1).at,
    };
  });

  return {
    range,
    generated_at: new Date(NOW).toISOString(),
    empty: false,
    kpis: { ...kpisOf(inRange), prev: prevList ? kpisOf(prevList) : null },
    clients: clientList,
    series: { bucket, points },
    most_read: mostRead,
    top_queries: tally(
      inRange.filter((e) => e.tool === "search_sessions").map((e) => e.input.query),
      10,
    ).map(([query, count]) => ({ query, count })),
    tools: tally(
      inRange.map((e) => e.tool),
      10,
    ).map(([name, count]) => ({ name, count })),
    recent: inRange
      .slice(-50)
      .reverse()
      .map((e) => ({
        at: e.at,
        client: e.client,
        client_name: friendly(e.client),
        tool: e.tool,
        input: e.input,
        session_ids: e.session_ids,
        count: e.count,
        total: e.total,
        duration_ms: e.duration_ms,
        error: e.error,
      })),
  };
}

const write = (name, data) =>
  writeFileSync(join(out, name), JSON.stringify(data, null, 2) + "\n");

for (const r of ["24h", "7d", "30d", "all"]) write(`stats-${r}.json`, stats(r));

// The log does not exist yet: every range looks like this.
const emptyHourly = [];
for (let i = 24; i >= 0; i--)
  emptyHourly.push({
    t: new Date(NOW - (NOW % HOUR) - i * HOUR).toISOString(),
    covered: false,
    by_client: {},
  });
write("stats-empty.json", {
  range: "24h",
  generated_at: new Date(NOW).toISOString(),
  empty: true,
  kpis: {
    calls: 0,
    sessions_read: 0,
    clients: 0,
    errors: 0,
    p50_ms: null,
    prev: { calls: 0, sessions_read: 0, clients: 0, errors: 0, p50_ms: null },
  },
  clients: [],
  series: { bucket: "hour", points: emptyHourly },
  most_read: [],
  top_queries: [],
  tools: [],
  recent: [],
});

// /api/session?id= for every Session that some call returned.
const detail = {};
for (const s of sessions) {
  const calls = entries
    .filter((e) => !e.error && e.session_ids.includes(s.id))
    .reverse()
    .map((e) => ({
      at: e.at,
      client_name: friendly(e.client),
      tool: e.tool,
      count: e.count,
    }));
  if (!calls.length) continue;
  detail[s.id] = {
    id: s.id,
    title: s.title || s.id.slice(0, 8),
    account: s.account,
    project: s.project,
    calls,
  };
}
write("sessions.json", detail);

console.log(`${entries.length} calls, ${Object.keys(detail).length} sessions`);
