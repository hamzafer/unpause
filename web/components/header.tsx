"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ThemeToggle } from "@/components/theme-toggle";
import { RANGES } from "@/lib/format";
import type { Range } from "@/lib/types";

export function Header({
  range,
  onRange,
}: {
  range: Range;
  onRange: (r: Range) => void;
}) {
  return (
    <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
      <div className="flex items-baseline gap-2">
        <span className="text-base font-semibold tracking-tight">unpause</span>
        <span className="text-sm text-muted-foreground">MCP access</span>
      </div>
      <div className="flex items-center gap-1">
        <ToggleGroup
          type="single"
          value={range}
          onValueChange={(v) => v && onRange(v as Range)}
          aria-label="Time range"
          variant="outline"
          size="sm"
          spacing={0}
          className="bg-card"
        >
          {RANGES.map((r) => (
            <ToggleGroupItem
              key={r.value}
              value={r.value}
              aria-label={r.value === "all" ? "All time" : `Last ${r.label}`}
              className="h-8 px-3 font-mono text-xs text-muted-foreground hover:bg-step hover:text-foreground data-[state=on]:bg-step data-[state=on]:text-foreground"
            >
              {r.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <ThemeToggle />
      </div>
    </header>
  );
}
