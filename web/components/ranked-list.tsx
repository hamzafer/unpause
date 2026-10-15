import { cn } from "cn";
import { Panel } from "@/components/panel";
import { num } from "@/lib/format";

export type RankedItem = {
  key: string;
  label: React.ReactNode;
  /** Plain text for the accessible name and the hover title. */
  text: string;
  sub?: React.ReactNode;
  value: number;
  dot?: string;
  onSelect?: () => void;
};

/** Vercel-style ranked list: a proportional bar behind each row, text in
 *  text colors, the value in mono on the right. */
export function RankedList({
  id,
  title,
  unit,
  items,
  empty,
}: {
  id: string;
  title: string;
  unit: string;
  items: RankedItem[];
  empty: string;
}) {
  const max = Math.max(1, ...items.map((i) => i.value));
  return (
    <Panel id={id} title={title} action={<span className="text-xs text-muted-foreground">{unit}</span>}>
      {items.length === 0 ? (
        <p className="px-4 pt-1 pb-4 text-sm text-muted-foreground">{empty}</p>
      ) : (
        <ol className="flex flex-col gap-1 px-2 pb-3">
          {items.map((it) => {
            const inner = (
              <>
                <span
                  aria-hidden
                  className="absolute inset-y-0 left-0 rounded-md bg-bar"
                  style={{ width: `${Math.max(2, (it.value / max) * 100)}%` }}
                />
                <span className="relative flex min-w-0 flex-1 items-center gap-2">
                  {it.dot && (
                    <span aria-hidden className="size-2 shrink-0 rounded-full" style={{ background: it.dot }} />
                  )}
                  <span className="min-w-0 truncate">
                    <span className="text-foreground">{it.label}</span>
                    {it.sub && <span className="ml-2 text-xs text-muted-foreground">{it.sub}</span>}
                  </span>
                </span>
                <span className="relative shrink-0 font-mono text-sm text-foreground tabular-nums">
                  {num(it.value)}
                </span>
              </>
            );
            const rowClass =
              "relative flex min-h-9 w-full items-center gap-3 rounded-md px-2 py-1.5 text-left text-sm";
            return (
              <li key={it.key} title={it.text}>
                {it.onSelect ? (
                  <button
                    type="button"
                    onClick={it.onSelect}
                    className={cn(rowClass, "cursor-pointer hover:bg-step/70")}
                    aria-label={`${it.text}, ${it.value} ${unit}. Show details`}
                  >
                    {inner}
                  </button>
                ) : (
                  <div className={rowClass}>{inner}</div>
                )}
              </li>
            );
          })}
        </ol>
      )}
    </Panel>
  );
}
