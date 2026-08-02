import { describe, expect, it } from "vitest";
import {
  BROWSER_RECENT_KEY,
  clearRecent,
  getRecent,
  recordVisit,
} from "./browserRecent";

function makeStorage(initial: Record<string, string> = {}): Storage {
  const m = new Map(Object.entries(initial));
  return {
    getItem: (k) => m.get(k) ?? null,
    setItem: (k, v) => { m.set(k, v); },
    removeItem: (k) => { m.delete(k); },
    clear: () => m.clear(),
    key: (i) => Array.from(m.keys())[i] ?? null,
    get length() { return m.size; },
  } as Storage;
}

describe("browserRecent", () => {
  it("starts empty and tolerates corrupt data", () => {
    const s = makeStorage();
    expect(getRecent(s)).toEqual([]);

    const corrupt = makeStorage({ [BROWSER_RECENT_KEY]: "{oops" });
    expect(getRecent(corrupt)).toEqual([]);

    const wrongShape = makeStorage({ [BROWSER_RECENT_KEY]: JSON.stringify({ a: 1 }) });
    expect(getRecent(wrongShape)).toEqual([]);
  });

  it("records a visit at the front with host and url", () => {
    const s = makeStorage();
    const next = recordVisit(s, "https://a.example/page", 1000);
    expect(next).toEqual([{ host: "a.example", url: "https://a.example/page", at: 1000 }]);
    expect(getRecent(s)).toEqual(next);
  });

  it("moves an existing host to the front and updates url/time", () => {
    const s = makeStorage();
    recordVisit(s, "https://a.example/one", 1000);
    recordVisit(s, "https://b.example/x", 2000);
    const next = recordVisit(s, "https://a.example/two", 3000);
    expect(next).toEqual([
      { host: "a.example", url: "https://a.example/two", at: 3000 },
      { host: "b.example", url: "https://b.example/x", at: 2000 },
    ]);
  });

  it("caps the list at max entries", () => {
    const s = makeStorage();
    recordVisit(s, "https://a.example", 1);
    recordVisit(s, "https://b.example", 2);
    recordVisit(s, "https://c.example", 3);
    const next = recordVisit(s, "https://d.example", 4, 2);
    expect(next).toEqual([
      { host: "d.example", url: "https://d.example", at: 4 },
      { host: "c.example", url: "https://c.example", at: 3 },
    ]);
  });

  it("ignores non-http(s) urls and clears the list", () => {
    const s = makeStorage();
    recordVisit(s, "https://a.example", 1);
    recordVisit(s, "not a url", 2);
    expect(getRecent(s).map((r) => r.host)).toEqual(["a.example"]);

    clearRecent(s);
    expect(getRecent(s)).toEqual([]);
  });
});
