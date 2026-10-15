// The JSON contract of `unpause mcp stats --web`
// (docs/superpowers/specs/2026-10-15-mcp-stats-web-design.md).

export type Range = "24h" | "7d" | "30d" | "all";

export type Kpis = {
  calls: number;
  sessions_read: number;
  clients: number;
  errors: number;
  p50_ms: number | null;
};

export type ClientStat = {
  id: string;
  name: string;
  slot: number;
  calls: number;
  /** Distinct Sessions this client read with a successful get_session. */
  sessions_read: number;
  last_seen: string;
};

export type SeriesPoint = {
  t: string;
  covered: boolean;
  by_client: Record<string, number>;
};

export type MostRead = {
  id: string;
  title: string;
  account: string;
  project: string;
  reads: number;
  last_read: string;
};

export type RecentCall = {
  at: string;
  client: string;
  client_name: string;
  tool: string;
  input: Record<string, unknown> | null;
  session_ids: string[] | null;
  count: number;
  total: number;
  duration_ms: number;
  error: string;
};

export type Stats = {
  range: Range;
  generated_at: string;
  empty: boolean;
  kpis: Kpis & { prev: Kpis | null };
  clients: ClientStat[];
  series: { bucket: "hour" | "day"; points: SeriesPoint[] };
  most_read: MostRead[];
  top_queries: { query: string; count: number }[];
  tools: { name: string; count: number }[];
  recent: RecentCall[];
};

export type SessionDetail = {
  id: string;
  title: string;
  account: string;
  project: string;
  calls: { at: string; client_name: string; tool: string; count: number }[];
};
