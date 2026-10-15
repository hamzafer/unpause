import { isOther, seriesVar } from "./colors";
import type { Stats } from "./types";

export type Series = { key: string; name: string; color: string; calls: number };
export type Row = Record<string, number | string | boolean | null> & {
  t: string;
  covered: boolean;
  total: number | null;
};

/**
 * The hero chart's data: client series in slot order (slot -1, the 6th client
 * on, folds into "Other", drawn last) and one row per bucket. A bucket before
 * the first logged call has null values, so it draws as a gap, never a zero.
 */
export function buildSeries(stats: Stats): { series: Series[]; rows: Row[] } {
  const keyOf = new Map<string, string>();
  const series: Series[] = [];
  const bySlot = [...stats.clients].sort((a, b) => a.slot - b.slot);
  for (const c of bySlot) {
    const key = isOther(c.slot) ? "other" : `s${c.slot}`;
    keyOf.set(c.id, key);
    const found = series.find((s) => s.key === key);
    if (found) found.calls += c.calls;
    else
      series.push({
        key,
        name: key === "other" ? "Other" : c.name,
        color: seriesVar(c.slot),
        calls: c.calls,
      });
  }
  // Ids that show up in the series but not in clients (should not happen).
  for (const p of stats.series.points)
    for (const id of Object.keys(p.by_client))
      if (!keyOf.has(id)) {
        keyOf.set(id, "other");
        if (!series.some((s) => s.key === "other"))
          series.push({ key: "other", name: "Other", color: seriesVar(-1), calls: 0 });
      }
  series.sort((a, b) => (a.key === "other" ? 1 : b.key === "other" ? -1 : a.key.localeCompare(b.key)));

  const rows: Row[] = stats.series.points.map((p) => {
    const row: Row = { t: p.t, covered: p.covered, total: p.covered ? 0 : null };
    for (const s of series) row[s.key] = p.covered ? 0 : null; // null draws a gap
    if (p.covered)
      for (const [id, n] of Object.entries(p.by_client)) {
        const k = keyOf.get(id)!;
        row[k] = (row[k] as number) + n;
        row.total = (row.total ?? 0) + n;
      }
    return row;
  });
  return { series, rows };
}

/** An area needs a few points to read as a shape; with fewer, stacked bars show the same data honestly. */
export const isSparse = (rows: Row[]) => rows.filter((r) => r.covered).length < 3;
