import { describe, expect, it } from "vitest";
import { compactInput, delta } from "./format";

describe("delta", () => {
  it("shows no chip without a previous window or a value", () => {
    expect(delta(5, undefined)).toBeNull();
    expect(delta(null, 3)).toBeNull();
  });
  it("says new when the previous window was empty", () => {
    expect(delta(4, 0)).toBe("new");
    expect(delta(4, null)).toBe("new");
    expect(delta(0, 0)).toBeNull();
  });
  it("signs the percent change", () => {
    expect(delta(24, 3)).toBe("+700%");
    expect(delta(1, 2)).toBe("−50%");
    expect(delta(3, 3)).toBe("0%");
    expect(delta(0, 4)).toBe("−100%");
    expect(delta(13000, 10)).toBe("+129,900%");
  });
});

describe("compactInput", () => {
  it("is empty for no input", () => {
    expect(compactInput(null)).toBe("");
    expect(compactInput({})).toBe("");
  });
  it("quotes the query and puts it first", () => {
    expect(compactInput({ limit: 10, query: "csv import" })).toBe('"csv import" limit=10');
  });
  it("shortens a full id and keeps a prefix or name as given", () => {
    expect(compactInput({ id: "9ee240be-f236-4657-b1e2-0eb7b3c624b3", offset: 120 })).toBe("9ee240be offset=120");
    expect(compactInput({ id: "9ee2" })).toBe("9ee2");
  });
  it("drops empty values and writes filters as k=v", () => {
    expect(compactInput({ account: "work", project: "", live: true, x: null, tags: ["a"] })).toBe(
      'account=work live=true tags=["a"]',
    );
  });
});
