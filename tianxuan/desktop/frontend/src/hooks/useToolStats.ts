// 工具/技能使用统计 hook
import { useMemo } from "react";
import type { Item } from "../lib/store";

export function useToolStats(items: Item[]) {
  return useMemo(() => {
    const toolCounts: Record<string, number> = {};
    const skillCounts: Record<string, number> = {};
    for (const it of items) {
      if (it.kind === "tool") {
        const name = (it as any).name as string;
        toolCounts[name] = (toolCounts[name] || 0) + 1;
        // run_skill 调用解析出技能名，追踪每个技能的使用次数
        if (name === "run_skill" && (it as any).args) {
          try {
            const args = JSON.parse((it as any).args as string);
            const sn = args?.name ?? args?.skill;
            if (sn) skillCounts[sn] = (skillCounts[sn] || 0) + 1;
          } catch { /* ignore parse errors */ }
        }
      }
    }
    return { toolCounts, skillCounts };
  }, [items]);
}
