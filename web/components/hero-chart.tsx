"use client";

import { useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ReferenceArea,
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
import { bucketLabel, num } from "@/lib/format";
import { buildSeries, isSparse, type Row, type Series } from "@/lib/series";
import type { Stats } from "@/lib/types";

export function HeroChart({ stats, pulse }: { stats: Stats; pulse: number }) {
  const [view, setView] = useState<"chart" | "table">("chart");
  const { series, rows } = useMemo(() => buildSeries(stats), [stats]);
  const bucket = stats.series.bucket;
  const lastIdx = rows.findLastIndex((r) => r.covered);
  const last = lastIdx >= 0 ? rows[lastIdx] : null;
  const config = Object.fromEntries(series.map((s) => [s.key, { label: s.name }]));
  const sparse = isSparse(rows);
  const Chart = sparse ? BarChart : AreaChart;
  const gap = rows.filter((r) => !r.covered);

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
            aria-label={`Stacked ${sparse ? "bar" : "area"} chart of calls per ${bucket} by client. Use the Table toggle for the values.`}
          >
            <Chart data={rows} margin={{ top: 12, right: 14, bottom: 0, left: 0 }}>
              <defs>
                <pattern
                  id="gap-hatch"
                  width="6"
                  height="6"
                  patternUnits="userSpaceOnUse"
                  patternTransform="rotate(45)"
                >
                  <line x1="0" y1="0" x2="0" y2="6" stroke="var(--grid)" strokeWidth="2" />
                </pattern>
              </defs>
              <CartesianGrid vertical={false} stroke="var(--grid)" />
              {gap.length > 0 && (
                <ReferenceArea
                  x1={gap[0].t}
                  x2={rows.find((r) => r.covered)?.t ?? gap[gap.length - 1].t}
                  fill="url(#gap-hatch)"
                  fillOpacity={1}
                  strokeOpacity={0}
                  ifOverflow="visible"
                />
              )}
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
                cursor={sparse ? { fill: "var(--grid)", fillOpacity: 0.5 } : { stroke: "var(--muted-text)", strokeWidth: 1 }}
                isAnimationActive={false}
                content={<HeroTooltip series={series} bucket={bucket} />}
              />
              {series.map((s, i) =>
                sparse ? (
                  <Bar
                    key={s.key}
                    dataKey={s.key}
                    name={s.name}
                    stackId="calls"
                    fill={s.color}
                    stroke="var(--card)"
                    strokeWidth={2}
                    maxBarSize={56}
                    radius={i === series.length - 1 ? [4, 4, 0, 0] : 0}
                    isAnimationActive={false}
                  />
                ) : (
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
                ),
              )}
              {last && (
                // The live edge: the last point, or the top of the last bar in the sparse view.
                <ReferenceDot
                  x={last.t}
                  y={last.total ?? 0}
                  ifOverflow="visible"
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
                      {!sparse && (
                        <circle
                          cx={p.cx}
                          cy={p.cy}
                          r={4}
                          fill="var(--text)"
                          stroke="var(--card)"
                          strokeWidth={2}
                        />
                      )}
                    </g>
                  )}
                />
              )}
            </Chart>
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
