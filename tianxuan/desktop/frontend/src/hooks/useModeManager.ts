// 模式管理 hook — normal/plan/yolo + thinkLevel → 温度 + 主题 + 模型切换
import { useState, useCallback } from "react";
import type { Mode } from "../lib/types";
import { getTheme } from "../lib/theme";
import type { Theme } from "../lib/theme";
import { app } from "../lib/bridge";

const THINK_TEMPS: Record<string, number> = { fast: 0.1, normal: 0.3, deep: 0.7 };

export function useModeManager(
  setPlan: (on: boolean) => void,
  setBypass: (on: boolean) => void,
  setModel: (name: string) => Promise<void>,
) {
  const [mode, setMode] = useState<Mode>("normal");
  const [thinkLevel, setThinkLevel] = useState<"fast" | "normal" | "deep">("normal");
  const [themeNow, setTheme] = useState<Theme>(getTheme);
  const [switchingModel, setSwitchingModel] = useState(false);

  const applyMode = useCallback(
    (m: Mode) => {
      setMode(m);
      setPlan(m === "plan");
      setBypass(m === "yolo");
    },
    [setPlan, setBypass],
  );

  const cycleMode = useCallback(() => {
    applyMode(mode === "normal" ? "plan" : mode === "plan" ? "yolo" : "normal");
  }, [mode, applyMode]);

  const handleThinkLevelChange = useCallback(async (level: string) => {
    setThinkLevel(level as "fast" | "normal" | "deep");
    const temp = THINK_TEMPS[level] ?? 0.3;
    // 先读取当前设置，保留用户配置的 maxSteps 和 systemPrompt，只改 temperature
    try {
      const settings = await app.Settings();
      app.SetAgentParams(temp, settings.agent.maxSteps, settings.agent.systemPrompt).catch(() => {});
    } catch {
      // Settings 读取失败时回退：只设温度，步数和提示词用 0（后端会用默认值）
      app.SetAgentParams(temp, 0, "").catch(() => {});
    }
  }, []);

  const switchModel = useCallback(
    async (name: string) => {
      setSwitchingModel(true);
      await setModel(name);
      setSwitchingModel(false);
      if (mode === "plan") setPlan(true);
      else if (mode === "yolo") setBypass(true);
    },
    [setModel, mode, setPlan, setBypass],
  );

  return { mode, setMode, thinkLevel, themeNow, setTheme, switchingModel, applyMode, cycleMode, handleThinkLevelChange, switchModel };
}
