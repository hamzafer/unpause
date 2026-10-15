"use client";

import { Check, Copy } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { fetchSession, ForbiddenError, NotFoundError } from "@/lib/api";
import { callTime, num, shortId } from "@/lib/format";
import type { MostRead, SessionDetail } from "@/lib/types";

type State =
  | { kind: "loading" }
  | { kind: "ok"; data: SessionDetail }
  | { kind: "error"; message: string };

/** Drill-in for one Session: who read it and when. No chat content. */
export function SessionSheet({
  session,
  onClose,
}: {
  session: MostRead | null;
  onClose: () => void;
}) {
  return (
    <Sheet open={!!session} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full gap-0 bg-card sm:max-w-md">
        {/* Keyed by id: a new Session starts from a fresh loading state. */}
        {session && <SessionBody key={session.id} session={session} />}
      </SheetContent>
    </Sheet>
  );
}

function SessionBody({ session }: { session: MostRead }) {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [copied, setCopied] = useState(false);
  const id = session.id;

  useEffect(() => {
    const ctl = new AbortController();
    fetchSession(id, ctl.signal)
      .then((data) => setState({ kind: "ok", data }))
      .catch((e: unknown) => {
        if (ctl.signal.aborted) return;
        setState({
          kind: "error",
          message:
            e instanceof NotFoundError
              ? "No logged call returned this session."
              : e instanceof ForbiddenError
                ? "This page needs the link unpause printed. Run unpause mcp stats --web again."
                : "Could not load this session. Is unpause still running?",
        });
      });
    return () => ctl.abort();
  }, [id]);

  const command = `unpause open ${shortId(id)}`;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {}
  };

  const title = state.kind === "ok" ? state.data.title : session.title;
  const account = state.kind === "ok" ? state.data.account : session.account;
  const project = state.kind === "ok" ? state.data.project : session.project;

  return (
    <>
        <SheetHeader className="gap-2 p-5 pr-12">
          <SheetTitle className="text-base leading-snug font-semibold">{title}</SheetTitle>
          <SheetDescription asChild>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
              {account && (
                <>
                  <dt className="text-muted-foreground">Account</dt>
                  <dd className="text-foreground">{account}</dd>
                </>
              )}
              {project && (
                <>
                  <dt className="text-muted-foreground">Project</dt>
                  <dd className="text-foreground">{project}</dd>
                </>
              )}
              <dt className="text-muted-foreground">Id</dt>
              <dd className="truncate font-mono text-xs leading-5 text-foreground">{id}</dd>
            </dl>
          </SheetDescription>
          <div className="mt-2 flex items-center gap-2 rounded-md border bg-background py-1 pr-1 pl-3">
            <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">{command}</code>
            <Button
              variant="ghost"
              size="sm"
              onClick={copy}
              className="h-7 gap-1.5 px-2 text-xs text-muted-foreground hover:text-foreground"
              aria-label={copied ? "Copied" : `Copy ${command}`}
            >
              {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>
          <span className="sr-only" aria-live="polite">
            {copied ? "Command copied" : ""}
          </span>
        </SheetHeader>
        <Separator />
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          <h3 className="mb-3 text-sm font-medium">
            Calls that returned it
            {state.kind === "ok" && (
              <span className="ml-2 font-mono text-muted-foreground">{num(state.data.calls.length)}</span>
            )}
          </h3>
          {state.kind === "loading" && (
            <div className="flex flex-col gap-2">
              {Array.from({ length: 5 }, (_, i) => (
                <Skeleton key={i} className="h-6 w-full bg-step" />
              ))}
            </div>
          )}
          {state.kind === "error" && <p className="text-sm text-muted-foreground">{state.message}</p>}
          {state.kind === "ok" && (
            <ol className="flex flex-col">
              {state.data.calls.map((c, i) => (
                <li
                  key={`${c.at}-${i}`}
                  className="grid grid-cols-[auto_1fr_auto] items-baseline gap-x-3 border-b py-2 text-sm last:border-0"
                >
                  <time dateTime={c.at} className="font-mono text-xs text-muted-foreground tabular-nums">
                    {callTime(c.at)}
                  </time>
                  <span className="min-w-0 truncate">
                    {c.client_name}
                    <span className="ml-2 font-mono text-xs text-muted-foreground">{c.tool}</span>
                  </span>
                  <span className="font-mono text-xs text-muted-foreground tabular-nums">{num(c.count)}</span>
                </li>
              ))}
            </ol>
          )}
        </div>
    </>
  );
}
