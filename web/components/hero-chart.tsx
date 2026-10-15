"use client";

import { useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceDot,
  XAxis,
  YAxis,
} from "recharts";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Panel } from "@/components/panel";
import { isOther, seriesVar } from "@/lib/colors";
import { bucketLabel, num } from "@/lib/format";
import type { Stats } from "@/lib/types";

type Series = { key: string; name: string; color: string; calls: number };
type Row = Record<string, number | string | boolean | null> & {
  t: string;
  covered: boolean;
  total: number | null;
};

/** Client series in slot order; slot -1 (6th client on) folds into "Other". */
function buildSeries(stats: Stats) {
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

export function HeroChart({ stats, pulse }: { stats: Stats; pulse: number }) {
  const [view, setView] = useState<"chart" | "table">("chart");
  const { series, rows } = useMemo(() => buildSeries(stats), [stats]);
  const bucket = stats.series.bucket;
  const lastIdx = rows.findLastIndex((r) => r.covered);
  const last = lastIdx >= 0 ? rows[lastIdx] : null;
  const config = Object.fromEntries(series.map((s) => [s.key, { label: s.name }]));

  return (
    <Panel
      id="hero"
      title={`Calls per ${bucket} by client`}
      action={
        <ToggleGroup
          type="single"
          value={view}
          onValueChange={(v) => v && setView(v as "chart" | "table")}
          aria-label="Show as"
          variant="outline"
          size="sm"
          spacing={0}
        >
          {(["chart", "table"] as const).map((v) => (
            <ToggleGroupItem
              key={v}
              value={v}
              className="h-7 px-2.5 text-xs text-muted-foreground hover:bg-step hover:text-foreground data-[state=on]:bg-step data-[state=on]:text-foreground"
            >
              {v === "chart" ? "Chart" : "Table"}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      }
    >
      {series.length >= 2 && (
        <ul className="flex flex-wrap gap-x-4 gap-y-1.5 px-4 pb-1 text-xs" aria-label="Legend">
          {series.map((s) => (
            <li key={s.key} className="flex items-center gap-1.5">
              <span
                aria-hidden
                className="size-2.5 shrink-0 rounded-[2px]"
                style={{ background: s.color }}
              />
              <span className="text-foreground">{s.name}</span>
              <span className="font-mono text-muted-foreground tabular-nums">{num(s.calls)}</span>
            </li>
          ))}
        </ul>
      )}

      {view === "chart" ? (
        <div className="px-2 pt-2 pb-3">
          <ChartContainer
            config={config}
            className="aspect-auto h-[240px] w-full sm:h-[280px]"
            role="img"
            aria-label={`Stacked area chart of calls per ${bucket} by client. Use the Table toggle for the values.`}
          >
            <AreaChart data={rows} margin={{ top: 12, right: 14, bottom: 0, left: 0 }}>
              <CartesianGrid vertical={false} stroke="var(--grid)" />
              <XAxis
                dataKey="t"
                tickFormatter={(t: string) => bucketLabel(t, bucket)}
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={28}
                interval="preserveStartEnd"
                className="font-mono"
              />
              <YAxis
                allowDecimals={false}
                tickLine={false}
                axisLine={false}
                width={32}
                tickMargin={4}
                className="font-mono"
              />
              <ChartTooltip
                cursor={{ stroke: "var(--muted-text)", strokeWidth: 1 }}
                isAnimationActive={false}
                content={<HeroTooltip series={series} bucket={bucket} />}
              />
              {series.map((s) => (
                <Area
                  key={s.key}
                  dataKey={s.key}
                  name={s.name}
                  stackId="calls"
                  type="monotone"
                  fill={s.color}
                  fillOpacity={0.4}
                  stroke="var(--card)"
                  strokeWidth={2}
                  connectNulls={false}
                  isAnimationActive={false}
                  activeDot={false}
                />
              ))}
              {last && (
                <ReferenceDot
                  x={last.t}
                  y={last.total ?? 0}
                  shape={(p: { cx?: number; cy?: number }) => (
                    <g>
                      {pulse > 0 && (
                        <circle
                          key={pulse}
                          className="live-pulse"
                          cx={p.cx}
                          cy={p.cy}
                          r={4}
                          fill="none"
                          stroke="var(--ui-accent)"
                          strokeWidth={2}
                        />
                      )}
                      <circle
                        cx={p.cx}
                        cy={p.cy}
                        r={4}
                        fill="var(--text)"
                        stroke="var(--card)"
                        strokeWidth={2}
                      />
                    </g>
                  )}
                />
              )}
            </AreaChart>
          </ChartContainer>
          {rows.some((r) => !r.covered) && (
            <p className="px-2 pt-1 text-xs text-muted-foreground">
              The gap is before the first logged call, not zero.
            </p>
          )}
        </div>
      ) : (
        <HeroTable rows={rows} series={series} bucket={bucket} />
      )}
    </Panel>
  );
}

function HeroTooltip({
  active,
  payload,
  series,
  bucket,
}: {
  active?: boolean;
  payload?: { payload: Row }[];
  series: Series[];
  bucket: "hour" | "day";
}) {
  if (!active || !payload?.length) return null;
  const row = payload[0].payload;
  return (
    <div className="min-w-44 rounded-md border bg-popover px-3 py-2 text-xs shadow-lg">
      <div className="mb-1.5 text-muted-foreground">{bucketLabel(row.t, bucket, true)}</div>
      {!row.covered ? (
        <div className="text-muted-foreground">Not logged yet</div>
      ) : (
        <div className="grid gap-1">
          {[...series].reverse().map((s) => (
            <div key={s.key} className="flex items-center gap-2">
              <span aria-hidden className="h-0.5 w-3 rounded-full" style={{ background: s.color }} />
              <span className="font-mono font-medium text-foreground tabular-nums">
                {num((row[s.key] as number) ?? 0)}
              </span>
              <span className="text-muted-foreground">{s.name}</span>
            </div>
          ))}
          <div className="mt-1 flex items-center gap-2 border-t pt-1.5">
            <span aria-hidden className="w-3" />
            <span className="font-mono font-medium text-foreground tabular-nums">{num(row.total ?? 0)}</span>
            <span className="text-muted-foreground">Total</span>
          </div>
        </div>
      )}
    </div>
  );
}

function HeroTable({
  rows,
  series,
  bucket,
}: {
  rows: Row[];
  series: Series[];
  bucket: "hour" | "day";
}) {
  return (
    <div className="max-h-[320px] overflow-auto border-t">
      <Table className="text-xs">
        <TableHeader className="sticky top-0 bg-card">
          <TableRow className="hover:bg-transparent">
            <TableHead className="px-4 font-normal text-muted-foreground">
              {bucket === "hour" ? "Hour" : "Day"}
            </TableHead>
            {series.map((s) => (
              <TableHead key={s.key} className="text-right font-normal text-muted-foreground">
                <span className="inline-flex items-center gap-1.5">
                  <span aria-hidden className="size-2 rounded-[2px]" style={{ background: s.color }} />
                  {s.name}
                </span>
              </TableHead>
            ))}
            <TableHead className="px-4 text-right font-normal text-muted-foreground">Total</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className="font-mono tabular-nums">
          {[...rows].reverse().map((r) => (
            <TableRow key={r.t} className="hover:bg-step/60">
              <TableCell className="px-4 text-muted-foreground">
                {bucketLabel(r.t, bucket, true)}
              </TableCell>
              {r.covered ? (
                <>
                  {series.map((s) => (
                    <TableCell key={s.key} className="text-right">
                      {num(r[s.key] as number)}
                    </TableCell>
                  ))}
                  <TableCell className="px-4 text-right font-medium">{num(r.total ?? 0)}</TableCell>
                </>
              ) : (
                <TableCell colSpan={series.length + 1} className="px-4 text-right font-sans text-muted-foreground">
                  Not logged yet
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
