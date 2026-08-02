import { describe, expect, it } from "vitest";
import { parseUnifiedDiff } from "./diffParser";

describe("parseUnifiedDiff", () => {
  it("returns empty for blank input", () => {
    expect(parseUnifiedDiff("")).toEqual([]);
    expect(parseUnifiedDiff("   ")).toEqual([]);
  });

  it("classifies headers, hunks, context, additions and deletions", () => {
    const lines = parseUnifiedDiff([
      "--- a/src/app.ts",
      "+++ b/src/app.ts",
      "@@ -1,3 +1,3 @@",
      " const a = 1;",
      "-const b = 2;",
      "+const b = 3;",
      " const c = 4;",
    ].join("\n"));
    expect(lines.map((l) => l.kind)).toEqual([
      "header", "header", "hunk",
      "context", "del", "add", "context",
    ]);
    expect(lines[0].text).toBe("a/src/app.ts");
    expect(lines[2].text).toBe("-1,3 +1,3");
  });

  it("tracks old and new line numbers across hunks", () => {
    const lines = parseUnifiedDiff([
      "@@ -1,3 +1,3 @@",
      " const a = 1;",
      "-const b = 2;",
      "+const b = 3;",
      "@@ -10,3 +10,4 @@",
      " func x() {",
      "+  return 1;",
      " }",
    ].join("\n"));
    const [hunk1, ctx1, del, add, hunk2, ctx2, add2, ctx3] = lines;
    expect(hunk1).toMatchObject({ kind: "hunk", text: "-1,3 +1,3" });
    expect(hunk2).toMatchObject({ kind: "hunk", text: "-10,3 +10,4" });
    expect(ctx1).toMatchObject({ kind: "context", oldLine: 1, newLine: 1 });
    expect(del).toMatchObject({ kind: "del", oldLine: 2 });
    expect(add).toMatchObject({ kind: "add", newLine: 2 });
    expect(ctx2).toMatchObject({ kind: "context", oldLine: 10, newLine: 10 });
    expect(add2).toMatchObject({ kind: "add", newLine: 11 });
    expect(ctx3).toMatchObject({ kind: "context", oldLine: 11, newLine: 12 });
  });

  it("treats lines outside any hunk as headers", () => {
    const lines = parseUnifiedDiff("--- a/x\n+++ b/x\nnot a diff line\n");
    expect(lines.map((l) => l.kind)).toEqual(["header", "header", "header"]);
    expect(lines[2].text).toBe("not a diff line");
  });
});
