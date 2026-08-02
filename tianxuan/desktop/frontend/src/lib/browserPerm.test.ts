import { describe, expect, it } from "vitest";
import {
  BROWSER_ALLOW_KEY,
  allowHost,
  getAllowedHosts,
  hostOf,
  isAllowedHost,
} from "./browserPerm";

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

describe("hostOf", () => {
  it("extracts the hostname of http(s) URLs", () => {
    expect(hostOf("https://Example.COM:8080/path?q=1")).toBe("example.com");
    expect(hostOf("http://localhost:3000")).toBe("localhost");
  });

  it("rejects non-http(s) or unparsable input", () => {
    expect(hostOf("file:///c:/x")).toBeNull();
    expect(hostOf("not a url")).toBeNull();
    expect(hostOf("")).toBeNull();
  });
});

describe("browser host allowlist", () => {
  it("starts empty and tolerates a missing or corrupt entry", () => {
    const s = makeStorage();
    expect(getAllowedHosts(s)).toEqual([]);
    expect(isAllowedHost("example.com", s)).toBe(false);

    const corrupt = makeStorage({ [BROWSER_ALLOW_KEY]: "{oops" });
    expect(getAllowedHosts(corrupt)).toEqual([]);

    const wrongShape = makeStorage({ [BROWSER_ALLOW_KEY]: JSON.stringify({ a: 1 }) });
    expect(getAllowedHosts(wrongShape)).toEqual([]);
  });

  it("allowHost persists the host and dedupes", () => {
    const s = makeStorage();
    allowHost("Example.COM", s);
    expect(getAllowedHosts(s)).toEqual(["example.com"]);
    expect(isAllowedHost("example.com", s)).toBe(true);

    allowHost("example.com", s);
    expect(getAllowedHosts(s)).toEqual(["example.com"]);
    expect(s.getItem(BROWSER_ALLOW_KEY)).toBe(JSON.stringify(["example.com"]));
  });

  it("merges with an existing allowlist", () => {
    const s = makeStorage({ [BROWSER_ALLOW_KEY]: JSON.stringify(["a.example"]) });
    allowHost("b.example", s);
    expect(getAllowedHosts(s)).toEqual(["a.example", "b.example"]);
  });
});
