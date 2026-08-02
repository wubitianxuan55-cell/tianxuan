import { describe, expect, it } from "vitest";
import {
  addComment,
  formatCommentsForAI,
  loadAllComments,
  loadPathComments,
  removeComment,
  type ReviewComment,
} from "./reviewComments";

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

describe("reviewComments", () => {
  it("starts empty and tolerates corrupt data", () => {
    const s = makeStorage();
    expect(loadAllComments(s, "/ws")).toEqual([]);

    const corrupt = makeStorage({ "tianxuan.reviewComments.%2Fws": "{oops" });
    expect(loadAllComments(corrupt, "/ws")).toEqual([]);
  });

  it("adds a comment per path and persists across loads", () => {
    const s = makeStorage();
    const added = addComment(s, "/ws", "src/a.ts", 12, "rename this");
    expect(added).toHaveLength(1);
    expect(added[0]).toMatchObject({ path: "src/a.ts", line: 12, text: "rename this" });
    expect(added[0].id).toBeTruthy();
    expect(loadPathComments(s, "/ws", "src/a.ts")).toEqual(added);
    expect(loadAllComments(s, "/ws")).toEqual(added);
  });

  it("scopes comments per workspace and per path", () => {
    const s = makeStorage();
    addComment(s, "/ws1", "a.ts", 1, "one");
    addComment(s, "/ws1", "b.ts", 2, "two");
    addComment(s, "/ws2", "a.ts", 3, "three");
    expect(loadPathComments(s, "/ws1", "a.ts").map((c) => c.text)).toEqual(["one"]);
    expect(loadAllComments(s, "/ws1").map((c) => c.text)).toEqual(["one", "two"]);
    expect(loadAllComments(s, "/ws2").map((c) => c.text)).toEqual(["three"]);
  });

  it("rejects blank text and removes by id", () => {
    const s = makeStorage();
    addComment(s, "/ws", "a.ts", 1, "   ");
    expect(loadAllComments(s, "/ws")).toEqual([]);

    const added = addComment(s, "/ws", "a.ts", 1, "keep");
    const rest = removeComment(s, "/ws", "a.ts", added[0].id);
    expect(rest).toEqual([]);
    expect(loadAllComments(s, "/ws")).toEqual([]);
  });

  it("formats comments for the AI", () => {
    const comments: ReviewComment[] = [
      { id: "1", path: "src/a.ts", line: 12, text: "rename this", at: 0 },
      { id: "2", path: "src/b.ts", line: 3, text: "overflow on mobile", at: 1 },
    ];
    expect(formatCommentsForAI(comments)).toBe(
      "src/a.ts:12: rename this\nsrc/b.ts:3: overflow on mobile",
    );
  });
});
