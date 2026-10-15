"use client";

import { CircleAlert } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "cn";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Panel } from "@/components/panel";
import { seriesVar } from "@/lib/colors";
import { callTime, compactInput, num } from "@/lib/format";
import type { RecentCall } from "@/lib/types";

const FIRST = 15;

export const callKey = (c: RecentCall, i: number) =>
  `${c.at}|${c.client}|${c.tool}|${i}`;

/** Identifies a call across polls (no index): calls in the same second differ in duration or count. */
export const freshKey = (c: RecentCall) =>
  `${c.at}|${c.client}|${c.tool}|${c.duration_ms}|${c.count}`;

export function RecentCalls({
  calls,
  slots,
  fresh,
}: {
  calls: RecentCall[];
  slots: Map<string, number>;
  fresh: Set<string>;
}) {
  const [all, setAll] = useState(false);
  const now = new Date();
  const shown = all ? calls : calls.slice(0, FIRST);
  return (
    <Panel
      id="recent"
      title="Recent calls"
      action={<span className="text-xs text-muted-foreground">Newest {calls.length}</span>}
    >
      {calls.length === 0 ? (
        <p className="px-4 pt-1 pb-4 text-sm text-muted-foreground">No calls in this range.</p>
      ) : (
        <div className="pb-2">
          <Table className="text-sm">
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                {["Time", "Client", "Tool", "Input", "Returned", "Duration", "Error"].map((h, i) => (
                  <TableHead
                    key={h}
                    className={cn(
                      "h-9 font-normal text-muted-foreground",
                      i === 0 && "pl-4",
                      i === 6 && "pr-4",
                      (h === "Returned" || h === "Duration") && "text-right",
                    )}
                  >
                    {h}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {shown.map((c, i) => {
                const key = callKey(c, i);
                const input = compactInput(c.input);
                const slot = slots.get(c.client);
                return (
                  <TableRow key={key} className={cn("hover:bg-step/60", fresh.has(freshKey(c)) && "row-new")}>
                    <TableCell className="pl-4 font-mono text-xs text-muted-foreground tabular-nums">
                      <time dateTime={c.at} title={new Date(c.at).toString()}>
                        {callTime(c.at, now)}
                      </time>
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-2">
                        <span
                          aria-hidden
                          className="size-2 shrink-0 rounded-full"
                          style={{ background: seriesVar(slot ?? -1) }}
                        />
                        <span title={c.client}>{c.client_name}</span>
                      </span>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{c.tool}</TableCell>
                    <TableCell className="max-w-[16rem] truncate font-mono text-xs text-muted-foreground" title={input}>
                      {input || " "}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {c.error ? (
                        <span className="text-muted-foreground">n/a</span>
                      ) : c.total && c.total !== c.count ? (
                        <>
                          {num(c.count)}
                          <span className="text-muted-foreground"> of {num(c.total)}</span>
                        </>
                      ) : (
                        num(c.count)
                      )}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs text-muted-foreground tabular-nums">
                      {num(c.duration_ms)} ms
                    </TableCell>
                    <TableCell className="pr-4 text-xs">
                      {c.error ? (
                        <span className="inline-flex items-center gap-1.5 text-foreground">
                          <CircleAlert aria-hidden className="size-3.5 text-danger" />
                          {c.error}
                        </span>
                      ) : (
                        <span className="sr-only">none</span>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
          {calls.length > FIRST && (
            <div className="border-t px-2 pt-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setAll((a) => !a)}
                aria-expanded={all}
                className="h-8 text-xs text-muted-foreground hover:text-foreground"
              >
                {all ? `Show the newest ${FIRST}` : `Show all ${calls.length}`}
              </Button>
            </div>
          )}
        </div>
      )}
    </Panel>
  );
}
