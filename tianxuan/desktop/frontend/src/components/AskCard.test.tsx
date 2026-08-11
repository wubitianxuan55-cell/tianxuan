// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { LocaleProvider } from "../lib/i18n";
import { AskCard } from "./AskCard";
import type { WireAsk, QuestionAnswer } from "../lib/types";

function wrap(ui: React.ReactNode) {
  return <LocaleProvider>{ui}</LocaleProvider>;
}

const singleAsk: WireAsk = {
  id: "ask-1",
  questions: [
    {
      id: "q1",
      header: "Scope",
      prompt: "How far should I go?",
      options: [
        { label: "Minimal", description: "Just the bug" },
        { label: "Full", description: "Refactor too" },
      ],
    },
  ],
};

const multiAsk: WireAsk = {
  id: "ask-2",
  questions: [
    {
      id: "q2",
      prompt: "Pick any",
      multi: true,
      options: [{ label: "A" }, { label: "B" }, { label: "C" }],
    },
  ],
};

const dualAsk: WireAsk = {
  id: "ask-3",
  questions: [
    { id: "q1", header: "Scope", prompt: "Scope?", options: [{ label: "S" }] },
    { id: "q2", header: "Speed", prompt: "Speed?", options: [{ label: "F" }] },
  ],
};

describe("AskCard 选项交互", () => {
  it("单选：点击选项后 Submit 启用，提交返回选中项", () => {
    const onAnswer = vi.fn();
    render(wrap(<AskCard ask={singleAsk} onAnswer={onAnswer} onDismiss={() => {}} />));
    const submit = screen.getByText("Submit") as HTMLButtonElement;
    expect(submit.disabled).toBe(true);

    fireEvent.click(screen.getByText("Full"));
    expect(submit.disabled).toBe(false);

    fireEvent.click(submit);
    expect(onAnswer).toHaveBeenCalledTimes(1);
    const [id, answers] = onAnswer.mock.calls[0] as [string, QuestionAnswer[]];
    expect(id).toBe("ask-1");
    expect(answers).toEqual([{ questionId: "q1", selected: ["Full"] }]);
  });

  it("单选：第二次点击替换而非追加", () => {
    const onAnswer = vi.fn();
    render(wrap(<AskCard ask={singleAsk} onAnswer={onAnswer} onDismiss={() => {}} />));
    fireEvent.click(screen.getByText("Minimal"));
    fireEvent.click(screen.getByText("Full"));
    fireEvent.click(screen.getByText("Submit"));
    const answers = onAnswer.mock.calls[0][1] as QuestionAnswer[];
    expect(answers[0].selected).toEqual(["Full"]);
  });

  it("多选：可累积多个，再点取消", () => {
    const onAnswer = vi.fn();
    render(wrap(<AskCard ask={multiAsk} onAnswer={onAnswer} onDismiss={() => {}} />));
    fireEvent.click(screen.getByText("A"));
    fireEvent.click(screen.getByText("C"));
    fireEvent.click(screen.getByText("Submit"));
    let answers = onAnswer.mock.calls[0][1] as QuestionAnswer[];
    expect(answers[0].selected).toEqual(["A", "C"]);

    fireEvent.click(screen.getByText("A"));
    fireEvent.click(screen.getByText("Submit"));
    answers = onAnswer.mock.calls[1][1] as QuestionAnswer[];
    expect(answers[0].selected).toEqual(["C"]);
  });

  it("多问题：全部回答后 Submit 才启用", () => {
    const onAnswer = vi.fn();
    render(wrap(<AskCard ask={dualAsk} onAnswer={onAnswer} onDismiss={() => {}} />));
    const submit = screen.getByText("Submit") as HTMLButtonElement;
    fireEvent.click(screen.getByText("S"));
    expect(submit.disabled).toBe(true);
    fireEvent.click(screen.getByText("F"));
    expect(submit.disabled).toBe(false);
    fireEvent.click(submit);
    const answers = onAnswer.mock.calls[0][1] as QuestionAnswer[];
    expect(answers).toEqual([
      { questionId: "q1", selected: ["S"] },
      { questionId: "q2", selected: ["F"] },
    ]);
  });
});

describe("AskCard 自定义输入", () => {
  it("输入自定义答案后清空选项，提交返回输入文本", () => {
    const onAnswer = vi.fn();
    render(wrap(<AskCard ask={singleAsk} onAnswer={onAnswer} onDismiss={() => {}} />));
    // 先选一个选项，再输入自定义——自定义应覆盖选项
    fireEvent.click(screen.getByText("Minimal"));
    fireEvent.change(screen.getByPlaceholderText("Type your own answer…"), {
      target: { value: "Custom plan" },
    });
    fireEvent.click(screen.getByText("Submit"));
    const answers = onAnswer.mock.calls[0][1] as QuestionAnswer[];
    expect(answers[0].selected).toEqual(["Custom plan"]);
  });

  it("空白自定义输入不视为已回答", () => {
    render(wrap(<AskCard ask={singleAsk} onAnswer={() => {}} onDismiss={() => {}} />));
    fireEvent.change(screen.getByPlaceholderText("Type your own answer…"), {
      target: { value: "   " },
    });
    expect((screen.getByText("Submit") as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("AskCard 提交与关闭", () => {
  it("Just chat 按钮调用 onDismiss", () => {
    const onDismiss = vi.fn();
    render(wrap(<AskCard ask={singleAsk} onAnswer={() => {}} onDismiss={onDismiss} />));
    fireEvent.click(screen.getByText("Just chat"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("全部回答后按 Enter 提交", () => {
    const onAnswer = vi.fn();
    render(wrap(<AskCard ask={singleAsk} onAnswer={onAnswer} onDismiss={() => {}} />));
    fireEvent.click(screen.getByText("Full"));
    fireEvent.keyDown(window, { key: "Enter" });
    expect(onAnswer).toHaveBeenCalledTimes(1);
  });

  it("未回答时按 Enter 不提交", () => {
    const onAnswer = vi.fn();
    render(wrap(<AskCard ask={singleAsk} onAnswer={onAnswer} onDismiss={() => {}} />));
    fireEvent.keyDown(window, { key: "Enter" });
    expect(onAnswer).not.toHaveBeenCalled();
  });

  it("多问题场景卡片标题不显示单问题 header", () => {
    render(wrap(<AskCard ask={dualAsk} onAnswer={() => {}} onDismiss={() => {}} />));
    // 多问题时每个 header 以内联形式出现，但卡片标题区只对单问题渲染
    expect(screen.getByText("Scope")).not.toBeNull();
    expect(screen.getByText("Speed")).not.toBeNull();
  });

  it("拖拽手柄存在且可触发 pointerdown（拖拽状态不崩溃）", () => {
    const { container } = render(
      wrap(<AskCard ask={singleAsk} onAnswer={() => {}} onDismiss={() => {}} />),
    );
    const handle = container.querySelector(".cursor-grab");
    expect(handle).not.toBeNull();
    act(() => {
      fireEvent.pointerDown(handle!, { button: 0, clientX: 100, clientY: 100 });
    });
    // 拖拽中不应抛错；卡片仍在
    expect(container.querySelector("button")).not.toBeNull();
  });
});
