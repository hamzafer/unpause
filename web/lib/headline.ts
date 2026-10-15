import { plural, RANGE_PHRASE } from "./format";
import type { Stats } from "./types";

/**
 * Sessions each client read (distinct ids from successful get_session).
 * Uses `clients[].sessions_read` when the server sends it; otherwise works it
 * out from `recent`, which is only possible when `recent` holds every call in
 * the range. Returns null when neither is available.
 */
export function readsByClient(s: Stats): Map<string, number> | null {
  const sent = s.clients.every(
    (c) => typeof (c as { sessions_read?: unknown }).sessions_read === "number",
  );
  if (sent && s.clients.length)
    return new Map(
      s.clients.map((c) => [c.id, (c as unknown as { sessions_read: number }).sessions_read]),
    );
  if (s.recent.length < s.kpis.calls) return null;
  const sets = new Map<string, Set<string>>();
  for (const c of s.clients) sets.set(c.id, new Set());
  for (const r of s.recent) {
    if (r.tool !== "get_session" || r.error) continue;
    const set = sets.get(r.client) ?? new Set<string>();
    for (const id of r.session_ids ?? []) set.add(id);
    sets.set(r.client, set);
  }
  return new Map([...sets].map(([k, v]) => [k, v.size]));
}

/** The answer, as one or two sentences. The page's one bold element. */
export function headline(s: Stats): string {
  if (s.empty) return "No MCP client has read your sessions yet.";
  const when = RANGE_PHRASE[s.range];
  const { calls, sessions_read } = s.kpis;
  if (calls === 0) return `No MCP client read your sessions ${when}.`;

  const clients = [...s.clients].sort((a, b) => b.calls - a.calls);
  const reads = readsByClient(s);

  if (reads && clients.length <= 3) {
    const readers = clients.filter((c) => (reads.get(c.id) ?? 0) > 0);
    const idle = clients.filter((c) => (reads.get(c.id) ?? 0) === 0);
    const first = readers.length
      ? readers
          .map((c, i) => {
            const n = plural(reads.get(c.id) ?? 0, "session");
            return i === 0 ? `${c.name} read ${n} ${when}` : `${c.name} read ${n}`;
          })
          .join(", ")
          .replace(/, ([^,]*)$/, " and $1") + "."
      : "";
    const rest = idle.length
      ? `${idle.map((c) => c.name).join(" and ")} ${readers.length ? "" : `made ${plural(calls, "call")} ${when} but `}read nothing.`
      : "";
    return [first, rest].filter(Boolean).join(" ");
  }

  if (clients.length === 1) {
    const c = clients[0];
    return sessions_read
      ? `${c.name} read ${plural(sessions_read, "session")} ${when}.`
      : `${c.name} made ${plural(calls, "call")} ${when} but read nothing.`;
  }

  const lead = clients[0];
  const who = `${clients.length} clients`;
  return sessions_read
    ? `${who} read ${plural(sessions_read, "session")} ${when}. ${lead.name} made the most calls.`
    : `${who} made ${plural(calls, "call")} ${when} but read nothing.`;
}
