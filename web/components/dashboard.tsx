"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { cn } from "cn";
import { EmptyState } from "@/components/empty-state";
import { Header } from "@/components/header";
import { HeroChart } from "@/components/hero-chart";
import { KpiRow } from "@/components/kpis";
import { RankedList } from "@/components/ranked-list";
import { freshKey, RecentCalls } from "@/components/recent-calls";
import { SessionSheet } from "@/components/session-sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { fetchStats, ForbiddenError } from "@/lib/api";
import { seriesVar } from "@/lib/colors";
import { relativeTime, shortId } from "@/lib/format";
import { headline } from "@/lib/headline";
import type { MostRead, Range, Stats } from "@/lib/types";

const POLL_MS = 5000;
const RANGE_KEY = "unpause-range";
const RANGE_VALUES: Range[] = ["24h", "7d", "30d", "all"];

// The remembered range is a per-viewer convenience kept in localStorage.
// Read through useSyncExternalStore so the prerendered HTML (7d) hydrates
// cleanly and the stored pick takes over right after.
const rangeListeners = new Set<() => void>();
function readRange(): Range {
  try {
    const r = localStorage.getItem(RANGE_KEY) as Range | null;
    if (r && RANGE_VALUES.includes(r)) return r;
  } catch {}
  return "7d";
}
function writeRange(r: Range) {
  try {
    localStorage.setItem(RANGE_KEY, r);
  } catch {}
  rangeListeners.forEach((f) => f());
}
function subscribeRange(f: () => void) {
  rangeListeners.add(f);
  return () => rangeListeners.delete(f);
}

export function Dashboard() {
  const range = useSyncExternalStore(subscribeRange, readRange, () => "7d" as Range);
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [forbidden, setForbidden] = useState(false);
  const [pulse, setPulse] = useState(0);
  const [fresh, setFresh] = useState<Set<string>>(new Set());
  const [open, setOpen] = useState<MostRead | null>(null);
  const newest = useRef<{ range: Range; at: string } | null>(null);
  // Every request gets the next number; only the newest one's answer lands, so
  // responses never apply out of order. pending is the number still out, if any.
  const seq = useRef(0);
  const pending = useRef(0);

  /** Fetch r. A poll (force false) is skipped while a request is out. */
  const load = useCallback(async (r: Range, signal: AbortSignal, force = false) => {
    if (pending.current && !force) return;
    const my = ++seq.current;
    pending.current = my;
    const current = () => !signal.aborted && my === seq.current;
    try {
      const s = await fetchStats(r, signal);
      if (!current()) return;
      const prev = newest.current;
      const top = s.recent[0]?.at ?? "";
      if (prev && prev.range === r && top > prev.at) {
        // Polling brought new calls: one pulse, tint the new rows.
        setPulse((p) => p + 1);
        setFresh(new Set(s.recent.filter((c) => c.at > prev.at).map(freshKey)));
      } else if (!prev || prev.range !== r) {
        setFresh(new Set());
      }
      newest.current = { range: r, at: top || prev?.at || "" };
      setStats(s);
      setFailed(false);
    } catch (e) {
      if (!current()) return;
      if (e instanceof ForbiddenError) setForbidden(true);
      else setFailed(true);
    } finally {
      if (pending.current === my) pending.current = 0;
      if (current()) setLoading(false);
    }
  }, []);

  // Fetch on range change, then poll every 5 s while the tab is visible.
  useEffect(() => {
    const ctl = new AbortController();
    // First fetch on the next tick, so the effect itself never sets state.
    const kick = setTimeout(() => load(range, ctl.signal, true), 0);
    let timer: ReturnType<typeof setInterval> | undefined;
    const start = () => {
      clearInterval(timer);
      timer = setInterval(() => load(range, ctl.signal), POLL_MS);
    };
    const onVis = () => {
      if (document.visibilityState === "visible") {
        load(range, ctl.signal, true);
        start();
      } else clearInterval(timer);
    };
    if (document.visibilityState === "visible") start();
    document.addEventListener("visibilitychange", onVis);
    return () => {
      ctl.abort();
      clearTimeout(kick);
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, [range, load]);

  const pickRange = (r: Range) => {
    if (r === range) return;
    setLoading(true); // dim the frame until the new range lands
    writeRange(r);
  };

  const slots = useMemo(
    () => new Map((stats?.clients ?? []).map((c) => [c.id, c.slot])),
    [stats],
  );

  return (
    <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-4 py-5 sm:px-6 sm:py-6">
      <Header range={range} onRange={pickRange} />

      {forbidden ? (
        <Forbidden />
      ) : !stats ? (
        failed ? (
          <Problem />
        ) : (
          <LoadingFrame />
        )
      ) : (
        <main
          className={cn("flex flex-col gap-8", loading && "opacity-60")}
          aria-busy={loading}
        >
          <section className="flex flex-col gap-6" aria-labelledby="headline">
            <div className="flex flex-col gap-2">
              <h1
                id="headline"
                className="max-w-4xl text-[26px] leading-[1.15] font-semibold tracking-[-0.03em] text-balance sm:text-[32px]"
              >
                {headline(stats)}
              </h1>
              <p className="text-sm text-muted-foreground" aria-live="polite">
                {failed ? (
                  <span className="text-foreground">Lost the connection to unpause. Retrying every 5 s.</span>
                ) : (
                  <>
                    Live.
                    {stats.recent[0] && <> Last call {relativeTime(stats.recent[0].at)}.</>}
                  </>
                )}
              </p>
            </div>
            {stats.empty ? (
              <EmptyState />
            ) : (
              <KpiRow kpis={stats.kpis} showDelta={stats.range !== "all"} />
            )}
          </section>

          {!stats.empty && (
            <>
              <HeroChart stats={stats} pulse={pulse} />

              <div className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2">
                <RankedList
                  id="most-read"
                  title="Most read sessions"
                  unit="reads"
                  empty="No session was read in this range."
                  items={stats.most_read.map((m) => ({
                    key: m.id,
                    label:
                      m.title && m.title !== shortId(m.id) ? (
                        m.title
                      ) : (
                        <span className="font-mono text-[13px]">{shortId(m.id)}</span>
                      ),
                    text: m.title || shortId(m.id),
                    sub: [m.account, m.project].filter(Boolean).join(" / "),
                    value: m.reads,
                    onSelect: () => setOpen(m),
                  }))}
                />
                <RankedList
                  id="queries"
                  title="Top queries"
                  unit="searches"
                  empty="No searches in this range."
                  items={stats.top_queries.map((q) => ({
                    key: q.query,
                    label: <span className="font-mono text-[13px]">{q.query}</span>,
                    text: q.query,
                    value: q.count,
                  }))}
                />
                <RankedList
                  id="clients"
                  title="Clients"
                  unit="calls"
                  empty="No client called in this range."
                  items={stats.clients.map((c) => ({
                    key: c.id,
                    label: c.name,
                    text: c.name === c.id ? c.name : `${c.name} (${c.id})`,
                    sub: `last ${relativeTime(c.last_seen)}`,
                    value: c.calls,
                    dot: seriesVar(c.slot),
                  }))}
                />
                <RankedList
                  id="tools"
                  title="Tools"
                  unit="calls"
                  empty="No calls in this range."
                  items={stats.tools.map((t) => ({
                    key: t.name,
                    label: <span className="font-mono text-[13px]">{t.name}</span>,
                    text: t.name,
                    value: t.count,
                  }))}
                />
              </div>

              <RecentCalls calls={stats.recent} slots={slots} fresh={fresh} />
            </>
          )}
        </main>
      )}

      <SessionSheet session={open} onClose={() => setOpen(null)} />
    </div>
  );
}

function LoadingFrame() {
  return (
    <div className="flex flex-col gap-6" aria-busy="true" aria-label="Loading">
      <Skeleton className="h-9 w-3/4 bg-step" />
      <Skeleton className="h-12 w-full bg-step" />
      <Skeleton className="h-72 w-full bg-step" />
    </div>
  );
}

function Forbidden() {
  return (
    <div className="flex flex-col gap-2" role="alert">
      <h1 className="text-[26px] leading-tight font-semibold tracking-[-0.03em] sm:text-[32px]">
        This page needs the link unpause printed.
      </h1>
      <p className="text-sm text-muted-foreground">
        Run <code className="font-mono text-foreground">unpause mcp stats --web</code> again and use the link it
        opens.
      </p>
    </div>
  );
}

function Problem() {
  return (
    <div className="flex flex-col gap-2">
      <h1 className="text-[26px] leading-tight font-semibold tracking-[-0.03em] sm:text-[32px]">
        Can&apos;t reach unpause.
      </h1>
      <p className="text-sm text-muted-foreground">
        This page reads from <code className="font-mono">unpause mcp stats --web</code>. Start it again and
        this page picks up within 5 s.
      </p>
    </div>
  );
}
