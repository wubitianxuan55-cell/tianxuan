// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import { fireEvent, render } from "@testing-library/react";
import { LocaleProvider } from "../lib/i18n";
import { ToolCard } from "./ToolCard";
import type { Item } from "../lib/store";

vi.mock("../lib/useGSAPCollapse", () => ({
  useGSAPCollapse: () => {},
}));

type ToolItem = Extract<Item, { kind: "tool" }>;

function wrap(ui: React.ReactNode) {
  return <LocaleProvider>{ui}</LocaleProvider>;
}

const runningItem: ToolItem = {
  kind: "tool",
  id: "t1",
  name: "bash",
  args: "sleep 1",
  status: "running",
  output: "",
  readOnly: false,
};

const doneItem: ToolItem = {
  kind: "tool",
  id: "t2",
  name: "bash",
  args: "echo hi",
  status: "done",
  output: "hi",
  readOnly: false,
};

describe("ToolCard 耗时显示", () => {
  it("运行中不显示耗时（有意设计：避免每秒重渲染）", () => {
    const { container } = render(wrap(<ToolCard item={runningItem} />));
    expect(container.querySelector(".tool__duration")).toBeNull();
  });

  it("完成后显示耗时（startedAtRef 在 running 时记录）", () => {
    const { container, rerender } = render(wrap(<ToolCard item={runningItem} />));
    // 模拟工具完成：同一组件实例 rerender 为 done
    rerender(wrap(<ToolCard item={doneItem} />));
    const duration = container.querySelector(".tool__duration");
    expect(duration).not.toBeNull();
    expect(duration!.textContent).toMatch(/\d+ ms/);
  });
});

describe("ToolCard 折叠", () => {
  it("默认折叠，点击头部展开", () => {
    const { container } = render(wrap(<ToolCard item={doneItem} />));
    expect(container.querySelector(".tool--open")).toBeNull();
    fireEvent.click(container.querySelector(".tool__head")!);
    expect(container.querySelector(".tool--open")).not.toBeNull();
  });

  it("状态图标：done 显示 ✓", () => {
    const { container } = render(wrap(<ToolCard item={doneItem} />));
    expect(container.querySelector(".tool__status-icon--ok")).not.toBeNull();
  });

  it("状态图标：running 无错误/成功图标", () => {
    const { container } = render(wrap(<ToolCard item={runningItem} />));
    expect(container.querySelector(".tool__status-icon--ok")).toBeNull();
    expect(container.querySelector(".tool__status-icon--err")).toBeNull();
  });
});
