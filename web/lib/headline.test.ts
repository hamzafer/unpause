import { describe, expect, it } from "vitest";
import { headline } from "./headline";
import { client, stats } from "./test-stats";

const kpis = (calls: number, sessions_read: number) => ({
  calls,
  sessions_read,
  clients: 0,
  errors: 0,
  p50_ms: null,
  prev: null,
});

describe("headline", () => {
  it("says nothing was read when the log is empty", () => {
    expect(headline(stats({ empty: true }))).toBe("No MCP client has read your sessions yet.");
  });

  it("says no client read anything in a quiet range", () => {
    expect(headline(stats({ range: "24h" }))).toBe(
      "No MCP client read your sessions in the last 24 hours.",
    );
  });

  it("names one client and what it read", () => {
    const s = stats({ kpis: kpis(5, 2), clients: [client("c", "Claude Cowork", 0, 5, 2)] });
    expect(headline(s)).toBe("Claude Cowork read 2 sessions in the last 7 days.");
  });

  it("names one client that only called", () => {
    const s = stats({ kpis: kpis(4, 0), clients: [client("c", "Codex", 0, 4, 0)] });
    expect(headline(s)).toBe("Codex made 4 calls in the last 7 days but read nothing.");
  });

  it("gives each reader its own count, never a summed total", () => {
    // Both read s1: 2 distinct sessions overall, but 2 + 1 per client.
    const s = stats({
      kpis: kpis(9, 2),
      clients: [client("a", "Claude Cowork", 0, 6, 2), client("b", "ChatGPT", 1, 3, 1)],
    });
    const h = headline(s);
    expect(h).toBe("Claude Cowork read 2 sessions in the last 7 days and ChatGPT read 1 session.");
    expect(h).not.toMatch(/3 sessions/);
  });

  it("names readers first, then the clients that read nothing", () => {
    const s = stats({
      kpis: kpis(9, 1),
      clients: [
        client("a", "Codex", 0, 2, 0),
        client("b", "Claude Cowork", 1, 6, 1),
        client("c", "ChatGPT", 2, 1, 0),
      ],
    });
    expect(headline(s)).toBe(
      "Claude Cowork read 1 session in the last 7 days. Codex and ChatGPT read nothing.",
    );
  });

  it("lists up to three idle clients with commas", () => {
    const s = stats({
      kpis: kpis(6, 0),
      clients: [client("a", "A", 0, 3), client("b", "B", 1, 2), client("c", "C", 2, 1)],
    });
    expect(headline(s)).toBe("A, B and C made 6 calls in the last 7 days but read nothing.");
  });

  it("sums up four or more clients with the true distinct total", () => {
    const cs = [client("a", "A", 0, 9, 3), client("b", "B", 1, 5, 2), client("c", "C", 2, 3, 1), client("d", "D", 3, 1, 1)];
    expect(headline(stats({ range: "all", kpis: kpis(18, 4), clients: cs }))).toBe(
      "4 clients read 4 sessions since the log began. A made the most calls.",
    );
    const idle = cs.map((c) => ({ ...c, sessions_read: 0 }));
    expect(headline(stats({ kpis: kpis(18, 0), clients: idle }))).toBe(
      "4 clients made 18 calls in the last 7 days but read nothing.",
    );
  });
});
