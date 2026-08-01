import { describe, it, expect } from "vitest";
import { isNearBottom, shouldFollowAfterGrow, distanceToBottom, BOTTOM_THRESHOLD_PX } from "./scrollFollow";

describe("distanceToBottom", () => {
  it("计算距底部距离 = scrollHeight - scrollTop - clientHeight", () => {
    expect(distanceToBottom(0, 1000, 500)).toBe(500);   // 顶部，距底 500
    expect(distanceToBottom(500, 1000, 500)).toBe(0);    // 正好到底
    expect(distanceToBottom(900, 1000, 500)).toBe(-400); // 已超出（距底为负）
  });
});

describe("isNearBottom", () => {
  it("阈值内（< 80px）视为贴底", () => {
    // scrollHeight=1000, clientHeight=500 → 距底 80 时 scrollTop = 1000-500-80 = 420
    expect(isNearBottom(430, 1000, 500)).toBe(true);   // 距底 70 < 80
    expect(isNearBottom(500, 1000, 500)).toBe(true);   // 距底 0
    expect(isNearBottom(421, 1000, 500)).toBe(true);   // 距底 79
    expect(isNearBottom(420, 1000, 500)).toBe(false);  // 距底 80，80 < 80 = false
    expect(isNearBottom(0, 1000, 500)).toBe(false);    // 距底 500
  });
  it("支持自定义阈值", () => {
    expect(isNearBottom(400, 1000, 500, 120)).toBe(true);  // 距底 100 < 120
    expect(isNearBottom(400, 1000, 500, 60)).toBe(false);  // 距底 100 >= 60
  });
});

describe("shouldFollowAfterGrow", () => {
  it("stick 为 false（用户已滚离）时绝不跟随", () => {
    expect(shouldFollowAfterGrow(false, 0, 1000, 500)).toBe(false);
    expect(shouldFollowAfterGrow(false, 430, 1000, 500)).toBe(false);
    expect(shouldFollowAfterGrow(false, 500, 1000, 500)).toBe(false);
  });

  it("stick 为 true 且真实位置贴底时跟随", () => {
    expect(shouldFollowAfterGrow(true, 430, 1000, 500)).toBe(true);
    expect(shouldFollowAfterGrow(true, 500, 1000, 500)).toBe(true);
  });

  // 关键回归场景：流式输出中 rAF 抢在 React onScroll 前执行。用户已向上滚动
  // （真实 scrollTop 离开底部），但 stick 仍是 true（onScroll 未及更新）。
  // 修复前 rAF 无条件拽回底部；修复后必须拒绝跟随。
  it("rAF 抢跑：stick 仍 true 但真实位置已离开底部 → 不跟随", () => {
    expect(shouldFollowAfterGrow(true, 300, 1000, 500)).toBe(false);  // 距底 200
    expect(shouldFollowAfterGrow(true, 100, 1000, 500)).toBe(false);  // 距底 400
    expect(shouldFollowAfterGrow(true, 0, 1000, 500)).toBe(false);    // 距底 500
  });

  it("内容增长后贴底跟随（新内容追加在底部）", () => {
    // 用户贴底（距底 10px），内容增长到 2000，真实位置距底仍 10
    expect(shouldFollowAfterGrow(true, 1490, 2000, 500)).toBe(true);
  });

  it("空内容 / 无滚动空间时不产生错误", () => {
    expect(shouldFollowAfterGrow(true, 0, 0, 0)).toBe(true);    // 空容器视为贴底
    expect(shouldFollowAfterGrow(false, 0, 0, 0)).toBe(false);
    expect(shouldFollowAfterGrow(true, 0, 100, 100)).toBe(true); // 无滚动空间
  });

  it("BOTTOM_THRESHOLD_PX 导出值稳定（组件与测试共用）", () => {
    expect(BOTTOM_THRESHOLD_PX).toBe(80);
  });
});
