import type { Range } from "./types";

export const RANGES: { value: Range; label: string }[] = [
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
  { value: "30d", label: "30d" },
  { value: "all", label: "All" },
];

export const RANGE_PHRASE: Record<Range, string> = {
  "24h": "in the last 24\u00a0hours",
  "7d": "in the last 7\u00a0days",
  "30d": "in the last 30\u00a0days",
  all: "since the log began",
};

const nf = new Intl.NumberFormat("en-US");
const compact = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

export const num = (n: number) => (n >= 10_000 ? compact.format(n) : nf.format(n));

export const plural = (n: number, one: string, many = one + "s") =>
  `${num(n)} ${n === 1 ? one : many}`;

export const shortId = (id: string) => id.slice(0, 8);

const pad = (n: number) => String(n).padStart(2, "0");
const MONTHS = "Jan Feb Mar Apr May Jun Jul Aug Sep Oct Nov Dec".split(" ");

/** Local 24-hour time; adds the day when it isn't today. */
export function callTime(iso: string, now = new Date()) {
  const d = new Date(iso);
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  const sameDay = d.toDateString() === now.toDateString();
  return sameDay ? time : `${MONTHS[d.getMonth()]} ${d.getDate()} ${time}`;
}

/** Axis and tooltip label for a series bucket. */
export function bucketLabel(iso: string, bucket: "hour" | "day", long = false) {
  const d = new Date(iso);
  if (bucket === "hour") {
    const h = `${pad(d.getHours())}:00`;
    return long ? `${MONTHS[d.getMonth()]} ${d.getDate()}, ${h}` : h;
  }
  return `${MONTHS[d.getMonth()]} ${d.getDate()}`;
}

export function relativeTime(iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
  if (s < 60) return "just now";
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} h ago`;
  const d = Math.round(h / 24);
  return `${d} d ago`;
}

/** Delta chip text: "+700%", "-50%", "0%", "new", or null for no chip. */
export function delta(cur: number | null, prev: number | null | undefined) {
  if (prev === undefined || cur === null) return null;
  if (prev === null || prev === 0) return cur && cur > 0 ? "new" : null;
  const pct = Math.round(((cur - prev) / prev) * 100);
  if (pct === 0) return "0%";
  return `${pct > 0 ? "+" : "−"}${nf.format(Math.abs(pct))}%`;
}

/** A call's input, compact: ids shortened, strings quoted, filters as k=v. */
export function compactInput(input: Record<string, unknown> | null) {
  if (!input) return "";
  const parts: string[] = [];
  for (const [k, v] of Object.entries(input)) {
    if (v === null || v === undefined || v === "") continue;
    if (k === "query" && typeof v === "string") parts.unshift(`"${v}"`);
    else if (k === "id" && typeof v === "string")
      parts.unshift(/^[0-9a-f-]{20,}$/i.test(v) ? shortId(v) : v);
    else parts.push(`${k}=${typeof v === "string" ? v : JSON.stringify(v)}`);
  }
  return parts.join(" ");
}
