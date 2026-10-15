import { plural, RANGE_PHRASE } from "./format";
import type { Stats } from "./types";

/** Sessions each client read (distinct ids from successful get_session), as the server counts them. */
export function readsByClient(s: Stats): Map<string, number> {
  return new Map(s.clients.map((c) => [c.id, c.sessions_read]));
}

/** The answer, as one or two sentences. The page's one bold element. */
export function headline(s: Stats): string {
  if (s.empty) return "No MCP client has read your sessions yet.";
  const when = RANGE_PHRASE[s.range];
  const { calls, sessions_read } = s.kpis;
  if (calls === 0) return `No MCP client read your sessions ${when}.`;

  const clients = [...s.clients].sort((a, b) => b.calls - a.calls);
  const reads = readsByClient(s);

  // Up to three clients: each one's own count. Clients can read the same
  // Session, so these are never added up into a total.
  if (clients.length <= 3) {
    const readers = clients.filter((c) => (reads.get(c.id) ?? 0) > 0);
    const idle = clients.filter((c) => (reads.get(c.id) ?? 0) === 0);
    const first = readers.length
      ? list(
          readers.map((c, i) => {
            const n = plural(reads.get(c.id) ?? 0, "session");
            return i === 0 ? `${c.name} read ${n} ${when}` : `${c.name} read ${n}`;
          }),
        ) + "."
      : "";
    const rest = idle.length
      ? `${list(idle.map((c) => c.name))} ${readers.length ? "" : `made ${plural(calls, "call")} ${when} but `}read nothing.`
      : "";
    return [first, rest].filter(Boolean).join(" ");
  }

  // Four or more: the distinct total from the server.
  const lead = clients[0];
  const who = `${clients.length} clients`;
  return sessions_read
    ? `${who} read ${plural(sessions_read, "session")} ${when}. ${lead.name} made the most calls.`
    : `${who} made ${plural(calls, "call")} ${when} but read nothing.`;
}

/** "a", "a and b", "a, b and c". */
function list(xs: string[]) {
  return xs.length < 2 ? xs.join("") : `${xs.slice(0, -1).join(", ")} and ${xs[xs.length - 1]}`;
}
