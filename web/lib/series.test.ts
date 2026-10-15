import { describe, expect, it } from "vitest";
import { buildSeries, isSparse } from "./series";
import { client, stats } from "./test-stats";
import type { SeriesPoint } from "./types";

const pt = (t: string, covered: boolean, by_client: Record<string, number> = {}): SeriesPoint => ({
  t,
  covered,
  by_client,
});

describe("buildSeries", () => {
  it("draws uncovered buckets as gaps (null), covered quiet ones as zero", () => {
    const s = stats({
      clients: [client("a", "A", 0, 3)],
      series: {
        bucket: "day",
        points: [pt("d1", false), pt("d2", true), pt("d3", true, { a: 3 })],
      },
    });
    const { series, rows } = buildSeries(s);
    expect(series.map((x) => x.key)).toEqual(["s0"]);
    expect(rows[0]).toMatchObject({ covered: false, total: null, s0: null });
    expect(rows[1]).toMatchObject({ covered: true, total: 0, s0: 0 });
    expect(rows[2]).toMatchObject({ covered: true, total: 3, s0: 3 });
  });

  it("folds slot -1 clients and unknown ids into Other, last", () => {
    const s = stats({
      clients: [client("x", "X", -1, 2), client("b", "B", 1, 1), client("a", "A", 0, 4), client("y", "Y", -1, 1)],
      series: { bucket: "hour", points: [pt("h1", true, { a: 4, b: 1, x: 2, y: 1, ghost: 5 })] },
    });
    const { series, rows } = buildSeries(s);
    expect(series.map((x) => [x.key, x.name, x.calls])).toEqual([
      ["s0", "A", 4],
      ["s1", "B", 1],
      ["other", "Other", 3],
    ]);
    expect(rows[0]).toMatchObject({ s0: 4, s1: 1, other: 8, total: 13 });
  });

  it("is sparse with fewer than three covered buckets", () => {
    const rows = (n: number) =>
      buildSeries(
        stats({
          series: {
            bucket: "day",
            points: [pt("a", false), ...Array.from({ length: n }, (_, i) => pt(`c${i}`, true))],
          },
        }),
      ).rows;
    expect(isSparse(rows(0))).toBe(true);
    expect(isSparse(rows(2))).toBe(true);
    expect(isSparse(rows(3))).toBe(false);
  });
});
