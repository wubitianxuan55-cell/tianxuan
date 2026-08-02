import { describe, expect, it } from "vitest";
import {
  canGoBack,
  canGoForward,
  createBrowserHistory,
  goBack,
  goForward,
  normalizeUrl,
  pushHistory,
} from "./browserHistory";

describe("browserHistory", () => {
  it("starts empty and cannot navigate", () => {
    const h = createBrowserHistory();
    expect(canGoBack(h)).toBe(false);
    expect(canGoForward(h)).toBe(false);
    expect(goBack(h)).toBe(h);
    expect(goForward(h)).toBe(h);
  });

  it("pushes a first entry and marks it current", () => {
    const h = pushHistory(createBrowserHistory(), "https://a.example");
    expect(h.entries).toEqual(["https://a.example"]);
    expect(h.index).toBe(0);
    expect(canGoBack(h)).toBe(false);
    expect(canGoForward(h)).toBe(false);
  });

  it("back and forward move through entries", () => {
    let h = pushHistory(createBrowserHistory(), "https://a.example");
    h = pushHistory(h, "https://b.example");
    h = pushHistory(h, "https://c.example");
    expect(h.entries).toEqual(["https://a.example", "https://b.example", "https://c.example"]);
    expect(canGoBack(h)).toBe(true);
    expect(canGoForward(h)).toBe(false);

    h = goBack(h);
    expect(h.index).toBe(1);
    expect(h.entries[h.index]).toBe("https://b.example");
    expect(canGoForward(h)).toBe(true);

    h = goBack(h);
    expect(h.index).toBe(0);
    expect(canGoBack(h)).toBe(false);

    h = goForward(h);
    expect(h.index).toBe(1);
  });

  it("pushing after back truncates the forward branch", () => {
    let h = pushHistory(createBrowserHistory(), "https://a.example");
    h = pushHistory(h, "https://b.example");
    h = pushHistory(h, "https://c.example");
    h = goBack(h);
    h = pushHistory(h, "https://d.example");
    expect(h.entries).toEqual(["https://a.example", "https://b.example", "https://d.example"]);
    expect(canGoForward(h)).toBe(false);
  });

  it("ignores duplicate current URL and blank input", () => {
    let h = pushHistory(createBrowserHistory(), "https://a.example");
    h = pushHistory(h, "https://a.example");
    expect(h.entries).toEqual(["https://a.example"]);
    h = pushHistory(h, "   ");
    expect(h.entries).toEqual(["https://a.example"]);
  });
});

describe("normalizeUrl", () => {
  it("prefixes https to bare hosts", () => {
    expect(normalizeUrl("example.com")).toBe("https://example.com");
    expect(normalizeUrl("localhost:3000")).toBe("https://localhost:3000");
  });

  it("keeps full URLs and protocol-relative URLs", () => {
    expect(normalizeUrl("http://example.com")).toBe("http://example.com");
    expect(normalizeUrl("https://example.com/path?q=1")).toBe("https://example.com/path?q=1");
    expect(normalizeUrl("//example.com")).toBe("https://example.com");
  });

  it("returns empty for blank input", () => {
    expect(normalizeUrl("")).toBe("");
    expect(normalizeUrl("   ")).toBe("");
  });
});
