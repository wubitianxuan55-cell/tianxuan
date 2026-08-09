// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { LocaleProvider } from "../lib/i18n";
import { Composer } from "./Composer";

// bridge 是 Go 内核接缝，测试中全 mock（列表类返回空，命令类记调用）
vi.mock("../lib/bridge", () => ({
  app: {
    Steer: vi.fn(),
    Cancel: vi.fn(),
    CaptureScreen: vi.fn(),
    Commands: vi.fn().mockResolvedValue([]),
    ListDir: vi.fn().mockResolvedValue({ entries: [], path: "" }),
    ListWorkspaces: vi.fn().mockResolvedValue([]),
    SavePastedImage: vi.fn(),
    AttachmentDataURL: vi.fn(),
    SlashArgs: vi.fn().mockResolvedValue({ items: [], from: 0 }),
  },
}));

function wrap(ui: React.ReactNode) {
  return <LocaleProvider>{ui}</LocaleProvider>;
}

// 挂载时 app.Commands()/ListWorkspaces() 等异步 resolve 会触发 act 警告，
// 渲染后 flush 一次微任务让它们完成
async function renderComposer(props: Partial<React.ComponentProps<typeof Composer>> = {}) {
  const utils = render(
    wrap(
      <Composer
        running={false}
        onSend={() => {}}
        onCancel={() => undefined}
        onPickFolder={async () => ""}
        {...props}
      />,
    ),
  );
  await act(async () => {});
  return utils;
}

function typeAndEnter(text: string, opts: { shiftKey?: boolean } = {}) {
  const ta = screen.getByRole("textbox");
  fireEvent.change(ta, { target: { value: text } });
  fireEvent.keyDown(ta, { key: "Enter", ...opts });
}

describe("Composer 发送", () => {
  it("非运行中输入 + Enter 直接发送", async () => {
    const onSend = vi.fn();
    await renderComposer({ onSend });
    typeAndEnter("hello world");
    expect(onSend).toHaveBeenCalledWith("hello world", "hello world");
    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it("空白文本不发送", async () => {
    const onSend = vi.fn();
    await renderComposer({ onSend });
    typeAndEnter("   ");
    expect(onSend).not.toHaveBeenCalled();
  });
});

describe("Composer 排队（running 中）", () => {
  it("Enter 只排队不发送，队列显示消息", async () => {
    const onSend = vi.fn();
    await renderComposer({ running: true, onSend });
    typeAndEnter("queued msg");
    expect(onSend).not.toHaveBeenCalled();
    expect(screen.getByText("queued msg")).toBeTruthy();
    expect(screen.getByText("Queued (1)")).toBeTruthy();
  });

  it("排队 2 条后取消第一条，第二条保留", async () => {
    await renderComposer({ running: true });
    typeAndEnter("first");
    typeAndEnter("second");
    expect(screen.getByText("first")).toBeTruthy();
    expect(screen.getByText("second")).toBeTruthy();
    const removeButtons = screen.getAllByTitle("Remove from queue");
    expect(removeButtons.length).toBe(2);
    fireEvent.click(removeButtons[0]);
    expect(screen.queryByText("first")).toBeNull();
    expect(screen.getByText("second")).toBeTruthy();
    expect(screen.getByText("Queued (1)")).toBeTruthy();
  });

  it("Shift+Enter 纠偏：取消当前轮次并记入 correctionRef", async () => {
    const onCancel = vi.fn().mockReturnValue(undefined);
    const onSend = vi.fn();
    await renderComposer({ running: true, onCancel, onSend });
    typeAndEnter("fix it", { shiftKey: true });
    expect(onCancel).toHaveBeenCalled();
    // 纠偏不直接 onSend（等 running→false 后自动发送）
    expect(onSend).not.toHaveBeenCalled();
    // 文本已清空
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
  });

  it("Escape 取消并清空队列", async () => {
    const onCancel = vi.fn();
    await renderComposer({ running: true, onCancel });
    typeAndEnter("q1");
    expect(screen.getByText("Queued (1)")).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Escape" });
    expect(onCancel).toHaveBeenCalled();
  });
});
