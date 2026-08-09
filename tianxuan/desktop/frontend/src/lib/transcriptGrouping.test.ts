import { describe, expect, it } from "vitest";
import { buildRenderSegments, warmPagination, warmUserPreview } from "./transcriptGrouping";
import type { Item } from "./store";

function userItem(id: string, text = "你好"): Item {
  return { kind: "user", id, text };
}

function assistantItem(id: string, text: string, reasoning = ""): Item {
  return { kind: "assistant", id, text, reasoning, streaming: false };
}

function toolItem(id: string, name = "bash"): Item {
  return { kind: "tool", id, name, args: "{}", readOnly: false, status: "done" };
}

function noticeItem(id: string, level: "info" | "warn", text: string): Item {
  return { kind: "notice", id, level, text };
}

describe("buildRenderSegments", () => {
  it("user 消息独立成段，assistant 思考进 process、正文进 outside", () => {
    const items: Item[] = [
      userItem("u1"),
      assistantItem("a1", "正文", "思考"),
    ];
    const segs = buildRenderSegments(items);
    expect(segs).toHaveLength(2);
    expect(segs[0]).toEqual({ processItems: [], outsideItems: [items[0]] });
    expect(segs[1].processItems).toEqual([{ ...items[1], text: "" }]);
    expect(segs[1].outsideItems).toEqual([{ ...items[1], reasoning: "" }]);
  });

  it("tool/compaction 进 process", () => {
    const items: Item[] = [
      userItem("u1"),
      toolItem("t1"),
      assistantItem("a1", "完成"),
    ];
    const segs = buildRenderSegments(items);
    const proc = segs.flatMap((s) => s.processItems);
    expect(proc.some((it) => it.kind === "tool" && it.id === "t1")).toBe(true);
  });

  it("warn notice（模型错误）进 outside，保证 UI 可见", () => {
    const items: Item[] = [
      userItem("u1"),
      noticeItem("e1", "warn", "Rate limit exceeded"),
    ];
    const segs = buildRenderSegments(items);
    const outside = segs.flatMap((s) => s.outsideItems);
    expect(outside.some((it) => it.kind === "notice" && it.level === "warn" && it.id === "e1")).toBe(true);
    const proc = segs.flatMap((s) => s.processItems);
    expect(proc.some((it) => it.kind === "notice" && it.id === "e1")).toBe(false);
  });

  it("info notice 保持进 process（不打扰正文）", () => {
    const items: Item[] = [
      userItem("u1"),
      noticeItem("n1", "info", "正在压缩对话"),
    ];
    const segs = buildRenderSegments(items);
    const proc = segs.flatMap((s) => s.processItems);
    expect(proc.some((it) => it.kind === "notice" && it.level === "info" && it.id === "n1")).toBe(true);
    const outside = segs.flatMap((s) => s.outsideItems);
    expect(outside.some((it) => it.kind === "notice" && it.id === "n1")).toBe(false);
  });
});

describe("warmPagination", () => {
  it("没有轮次时 warm 区为空", () => {
    expect(warmPagination({ turnCount: 0, hotTurns: 1, pageSize: 20, coldPage: 0 }))
      .toEqual({ warmStartTurn: 0, warmEndTurn: 0, coldTurnCount: 0 });
  });

  it("hotTurns=1 时最新一轮留在冷区（生成结束后正文/过程仍可见）", () => {
    expect(warmPagination({ turnCount: 1, hotTurns: 1, pageSize: 20, coldPage: 0 }))
      .toEqual({ warmStartTurn: 0, warmEndTurn: 0, coldTurnCount: 0 });
    expect(warmPagination({ turnCount: 2, hotTurns: 1, pageSize: 20, coldPage: 0 }))
      .toEqual({ warmStartTurn: 0, warmEndTurn: 1, coldTurnCount: 0 });
    expect(warmPagination({ turnCount: 5, hotTurns: 1, pageSize: 20, coldPage: 0 }))
      .toEqual({ warmStartTurn: 0, warmEndTurn: 4, coldTurnCount: 0 });
  });

  it("hotTurns=0 时最新一轮也被折叠（V10.175 输出不可见 bug 场景）", () => {
    expect(warmPagination({ turnCount: 2, hotTurns: 0, pageSize: 20, coldPage: 0 }))
      .toEqual({ warmStartTurn: 0, warmEndTurn: 2, coldTurnCount: 0 });
  });

  it("超过 pageSize 的旧轮按 coldPage 分页", () => {
    expect(warmPagination({ turnCount: 25, hotTurns: 1, pageSize: 20, coldPage: 0 }))
      .toEqual({ warmStartTurn: 4, warmEndTurn: 24, coldTurnCount: 4 });
    expect(warmPagination({ turnCount: 25, hotTurns: 1, pageSize: 20, coldPage: 1 }))
      .toEqual({ warmStartTurn: 0, warmEndTurn: 24, coldTurnCount: 0 });
  });
});

describe("warmUserPreview", () => {
  it("去掉附件引用并压缩空白", () => {
    expect(warmUserPreview("[[reasonix-attach:abc]]  你好  世界 "))
      .toBe("你好 世界");
  });

  it("超长文本截断到 80 字符", () => {
    const long = "长".repeat(100);
    expect(warmUserPreview(long)).toBe("长".repeat(77) + "...");
  });
});
