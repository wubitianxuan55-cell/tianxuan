// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { LocaleProvider } from "../lib/i18n";
import { Transcript } from "./Transcript";
import { useStore } from "../lib/store";
import type { Item } from "../lib/store";

// 动画/观察器在测试环境无实际意义，mock 掉避免 gsap/ResizeObserver 副作用
vi.mock("../lib/useGSAPCollapse", () => ({
  useGSAPCollapse: () => {},
}));
vi.mock("../lib/useEntranceAnimation", () => ({
  useEntranceAnimation: () => ({ current: null }),
}));

function wrap(ui: React.ReactNode) {
  return <LocaleProvider>{ui}</LocaleProvider>;
}

const THREE_TURNS: Item[] = [
  { kind: "user", id: "u0", text: "question one" },
  { kind: "assistant", id: "a0", text: "answer one", reasoning: "", streaming: false },
  { kind: "user", id: "u1", text: "question two" },
  { kind: "assistant", id: "a1", text: "answer two", reasoning: "", streaming: false },
  { kind: "user", id: "u2", text: "question three" },
  { kind: "assistant", id: "a2", text: "answer three", reasoning: "", streaming: false },
];

function renderTranscript(items: Item[], running = false) {
  useStore.setState({ items, running, turnStartAt: 0 });
  return render(
    wrap(<Transcript onPrompt={() => {}} running={running} />),
  );
}

afterEach(() => {
  // 还原 store，避免跨测试污染
  useStore.setState({ items: [], running: false, turnStartAt: 0 });
});

describe("Transcript 过程卡（warm-turn）", () => {
  it("3 轮时旧轮收进 warm 折叠列表，最新轮留在外面", () => {
    renderTranscript(THREE_TURNS);
    // 前 2 轮折叠成 warm-turn（每轮一行）
    const heads = document.querySelectorAll(".warm-turn__head");
    expect(heads.length).toBe(2);
    // 最新轮正文在外可见
    expect(screen.getByText("answer three")).toBeTruthy();
  });

  it("点击 warm-turn 头部展开单轮内容", () => {
    renderTranscript(THREE_TURNS);
    // 折叠时旧轮正文不可见
    expect(screen.queryByText("answer one")).toBeNull();
    fireEvent.click(document.querySelectorAll(".warm-turn__head")[0]);
    // 展开后该轮正文可见
    expect(screen.getByText("answer one")).toBeTruthy();
  });

  it("warm 列表不包含最新轮（question three 不在折叠区）", () => {
    renderTranscript(THREE_TURNS);
    const previews = Array.from(document.querySelectorAll(".warm-turn__preview"))
      .map((el) => el.textContent);
    expect(previews).toContain("question one");
    expect(previews).toContain("question two");
    expect(previews).not.toContain("question three");
  });
});

describe("TurnCollapse 自动展开/折叠（V10.175 过程卡）", () => {
  const TOOL_TURN: Item[] = [
    { kind: "user", id: "u0", text: "question" },
    { kind: "tool", id: "t0", name: "bash", args: "echo hi", status: "done", output: "hi", readOnly: false },
    { kind: "assistant", id: "a0", text: "answer", reasoning: "thinking…", streaming: false },
  ];

  it("running 时自动展开，结束后自动折叠（hotTurns=1 修复回归保护）", () => {
    useStore.setState({ items: TOOL_TURN, running: true, turnStartAt: 0 });
    const { rerender } = render(wrap(<Transcript onPrompt={() => {}} running />));
    const collapse = document.querySelector(".turn-collapse");
    expect(collapse).not.toBeNull();
    expect(collapse!.classList.contains("turn-collapse--open")).toBe(true);
    expect(
      collapse!.querySelector(".turn-collapse__head")?.getAttribute("aria-expanded"),
    ).toBe("true");

    // 生成结束 → 自动折叠
    rerender(wrap(<Transcript onPrompt={() => {}} running={false} />));
    const collapse2 = document.querySelector(".turn-collapse");
    expect(collapse2!.classList.contains("turn-collapse--open")).toBe(false);
    expect(
      collapse2!.querySelector(".turn-collapse__head")?.getAttribute("aria-expanded"),
    ).toBe("false");
  });
});
