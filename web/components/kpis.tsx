import { cn } from "cn";
import { delta, num } from "@/lib/format";
import type { Kpis } from "@/lib/types";

type Tile = {
  key: keyof Kpis;
  label: string;
  value: (k: Kpis) => string;
  /** Muted value when this is true (a zero that is good news, or no data). */
  quiet?: (k: Kpis) => boolean;
};

const TILES: Tile[] = [
  { key: "calls", label: "Calls", value: (k) => num(k.calls) },
  { key: "sessions_read", label: "Sessions read", value: (k) => num(k.sessions_read) },
  { key: "clients", label: "Clients", value: (k) => num(k.clients) },
  {
    key: "errors",
    label: "Errors",
    value: (k) => num(k.errors),
    quiet: (k) => k.errors === 0,
  },
  {
    key: "p50_ms",
    label: "p50",
    value: (k) => (k.p50_ms === null ? "n/a" : `${num(k.p50_ms)} ms`),
    quiet: (k) => k.p50_ms === null,
  },
];

/** Five figures with neutral delta chips (more calls is not good or bad). */
export function KpiRow({
  kpis,
  showDelta,
}: {
  kpis: Kpis & { prev: Kpis | null };
  showDelta: boolean;
}) {
  return (
    <dl className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-3 md:flex md:flex-wrap md:gap-x-10">
      {TILES.map((t) => {
        // Latency has no "new": with nothing before, there is nothing to compare.
        const noPrev = kpis.prev?.[t.key] === null || kpis.prev?.[t.key] === 0;
        const d =
          showDelta && kpis.prev && !(t.key === "p50_ms" && noPrev)
            ? delta(kpis[t.key], kpis.prev[t.key])
            : null;
        const quiet = t.quiet?.(kpis);
        return (
          <div key={t.key} className="flex min-w-0 flex-col gap-1">
            <dt className="text-sm text-muted-foreground">{t.label}</dt>
            <dd className="flex items-baseline gap-2">
              <span
                className={cn(
                  "font-mono text-xl font-medium tracking-tight",
                  quiet ? "text-muted-foreground" : "text-foreground",
                )}
              >
                {t.value(kpis)}
              </span>
              {d && (
                <span
                  className="rounded-sm bg-step px-1.5 py-0.5 font-mono text-xs text-muted-foreground"
                  title={
                    d === "new"
                      ? "None in the period before"
                      : `Change from the period before (${
                          kpis.prev?.[t.key] === null ? "n/a" : kpis.prev?.[t.key]
                        })`
                  }
                >
                  {d}
                </span>
              )}
            </dd>
          </div>
        );
      })}
    </dl>
  );
}
