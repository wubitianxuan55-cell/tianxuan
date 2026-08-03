import { describe, expect, it } from "vitest";
import {
  BROWSER_BOOKMARK_KEY,
  addBookmark,
  getBookmarks,
  isBookmarked,
  removeBookmark,
} from "./browserBookmarks";

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

describe("browserBookmarks", () => {
  it("starts empty and tolerates corrupt data", () => {
    const s = makeStorage();
    expect(getBookmarks(s)).toEqual([]);

    const corrupt = makeStorage({ [BROWSER_BOOKMARK_KEY]: "{oops" });
    expect(getBookmarks(corrupt)).toEqual([]);

    const wrongShape = makeStorage({ [BROWSER_BOOKMARK_KEY]: JSON.stringify({ a: 1 }) });
    expect(getBookmarks(wrongShape)).toEqual([]);
  });

  it("adds a bookmark at the front with host and addedAt", () => {
    const s = makeStorage();
    const next = addBookmark(s, "https://a.example/page", 1000);
    expect(next).toEqual([{ url: "https://a.example/page", host: "a.example", addedAt: 1000 }]);
    expect(getBookmarks(s)).toEqual(next);
    expect(isBookmarked(s, "https://a.example/page")).toBe(true);
  });

  it("dedupes by URL, moving the existing entry to the front and updating time", () => {
    const s = makeStorage();
    addBookmark(s, "https://a.example/one", 1000);
    addBookmark(s, "https://b.example/x", 2000);
    const next = addBookmark(s, "https://a.example/one", 3000);
    expect(next).toEqual([
      { url: "https://a.example/one", host: "a.example", addedAt: 3000 },
      { url: "https://b.example/x", host: "b.example", addedAt: 2000 },
    ]);
  });

  it("caps the list at max entries", () => {
    const s = makeStorage();
    addBookmark(s, "https://a.example", 1);
    addBookmark(s, "https://b.example", 2);
    addBookmark(s, "https://c.example", 3);
    const next = addBookmark(s, "https://d.example", 4, 2);
    expect(next).toEqual([
      { url: "https://d.example", host: "d.example", addedAt: 4 },
      { url: "https://c.example", host: "c.example", addedAt: 3 },
    ]);
  });

  it("removes a bookmark by url", () => {
    const s = makeStorage();
    addBookmark(s, "https://a.example/one", 1000);
    addBookmark(s, "https://b.example/x", 2000);
    const next = removeBookmark(s, "https://a.example/one");
    expect(next).toEqual([{ url: "https://b.example/x", host: "b.example", addedAt: 2000 }]);
    expect(isBookmarked(s, "https://a.example/one")).toBe(false);
    expect(isBookmarked(s, "https://b.example/x")).toBe(true);
  });

  it("ignores non-http(s) urls and unknown removals", () => {
    const s = makeStorage();
    const next = addBookmark(s, "file:///c:/tmp/x", 1000);
    expect(next).toEqual([]);

    addBookmark(s, "https://a.example", 1000);
    const after = removeBookmark(s, "https://nope.example");
    expect(after).toEqual([{ url: "https://a.example", host: "a.example", addedAt: 1000 }]);
  });
});
