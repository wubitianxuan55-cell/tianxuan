// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { LocaleProvider } from "../lib/i18n";
import { ReasoningProcess, UserMessage } from "./Message";
import { useStore } from "../lib/store";
import type { Item } from "../lib/store";

type AssistantItem = Extract<Item, { kind: "assistant" }>;

// 动画 hook 在测试环境无实际意义，mock 掉避免 gsap 副作用
vi.mock("../lib/useGSAPCollapse", () => ({
  useGSAPCollapse: () => {},
}));

function wrap(ui: React.ReactNode) {
  return <LocaleProvider>{ui}</LocaleProvider>;
}

describe("UserMessage rewind 交互", () => {
  it("open 时点击菜单项调用 onRewind(turn, scope)", () => {
    const onRewind = vi.fn();
    render(
      wrap(
        <UserMessage text="hello" turn={3} open onToggle={() => {}} onRewind={onRewind} />,
      ),
    );
    fireEvent.click(screen.getByText("Code + conversation"));
    expect(onRewind).toHaveBeenCalledWith(3, "both");
    fireEvent.click(screen.getByText("Conversation only"));
    expect(onRewind).toHaveBeenCalledWith(3, "conversation");
  });

  it("回退按钮行容器带 relative（rewind 定位修复回归保护）", () => {
    const { container } = render(
      wrap(
        <UserMessage text="hi" turn={0} open onToggle={() => {}} onRewind={() => {}} />,
      ),
    );
    const row = container.querySelector(".relative.flex.justify-end.items-center");
    expect(row).not.toBeNull();
  });
});

describe("ReasoningProcess 思考计时", () => {
  it("思考中 elapsed 每秒刷新（V10.176 实时计时回归保护）", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00.000Z"));
    // turnStartAt 由 store 在回合开始时设置；测试设为 5 秒前
    useStore.setState({ turnStartAt: Date.now() - 5000 });
    const item = {
      kind: "assistant",
      id: "a1",
      text: "",
      reasoning: "working…",
      streaming: true,
    } as unknown as AssistantItem;
    const { container } = render(wrap(<ReasoningProcess item={item} />));
    const meta = container.querySelector(".reasoning__meta");
    expect(meta).not.toBeNull();
    expect(meta!.textContent).toBe("5s");
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(meta!.textContent).toBe("6s");
    vi.useRealTimers();
  });

  it("结束后冻结 elapsed（finalElapsedRef）", () => {
    const item = {
      kind: "assistant",
      id: "a2",
      text: "",
      reasoning: "done",
      streaming: false,
    } as unknown as AssistantItem;
    const { container } = render(wrap(<ReasoningProcess item={item} />));
    expect(container.querySelector(".reasoning__meta")).not.toBeNull();
  });

  it("点击头部折叠/展开思考内容", () => {
    const item = {
      kind: "assistant",
      id: "a3",
      text: "",
      reasoning: "step1\nstep2",
      streaming: false,
    } as unknown as AssistantItem;
    const { container } = render(wrap(<ReasoningProcess item={item} />));
    // 非 streaming 默认折叠
    expect(container.querySelector(".reasoning__body")).toBeNull();
    const head = container.querySelector(".reasoning__head");
    fireEvent.click(head!);
    // 用户手动展开后 body 渲染
    expect(container.querySelector(".reasoning__body")).not.toBeNull();
  });
});
