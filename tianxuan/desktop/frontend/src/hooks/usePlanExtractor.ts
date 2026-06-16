// 计划内容提取 hook — 从 state.items 中提取计划 Markdown
// 优先级：<plan> 标签 > create_plan 工具 > 回退匹配（编号列表/H2标题）
import { useMemo } from "react";
import type { Item } from "../lib/store";

export function usePlanExtractor(items: Item[]): string {
  return useMemo(() => {
    // 路径 1：解析 <plan>...</plan> 标签（PlanModeMarker 指示 AI 使用）
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "assistant" && "text" in it && it.text) {
        const text = it.text as string;
        const m = text.match(/<plan>([\s\S]*?)<\/plan>/i);
        if (m?.[1]?.trim()) return m[1].trim();
      }
    }

    // 路径 2：create_plan 工具调用后的 assistant 消息
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "tool" && it.name === "create_plan") {
        for (let j = i + 1; j < items.length; j++) {
          const next = items[j];
          if (next.kind === "assistant" && "text" in next && next.text) return next.text as string;
          if (next.kind === "tool") break;
        }
      }
    }

    // 路径 3：回退 — 匹配编号列表开头（AI 在计划模式下的输出格式）
    // 或旧格式的 H2 标题
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "assistant" && "text" in it && it.text) {
        const text = it.text as string;
        // 编号列表 + 计划关键词
        if (/^\d+\.\s/.test(text) && /(?:plan|计划|方案|实施|步骤|phase|阶段)/i.test(text)) {
          return text;
        }
        // H2 标题格式（旧兼容）
        if (/^##\s+(?:Implementation|实施|Plan|计划)/m.test(text)) {
          return text;
        }
      }
    }
    return "";
  }, [items]);
}
