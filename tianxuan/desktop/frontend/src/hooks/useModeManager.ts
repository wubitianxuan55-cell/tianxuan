// 权限管理 hook — V10.19: 统一 ask/auto/yolo 三级权限
import { useState, useCallback } from "react";
import type { PermLevel } from "../lib/types";
import { getTheme } from "../lib/theme";
import type { Theme } from "../lib/theme";
import { app } from "../lib/bridge";

const THINK_TEMPS: Record<string, number> = { fast: 0.1, normal: 0.3, deep: 0.7 };

export function useModeManager(
  setPlan: (on: boolean) => void,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  _setBypass: (...args: any[]) => any,
  setModel: (name: string) => Promise<void>,
) {
  const [permLevel, setPermLevelState] = useState<PermLevel>("ask");
  const [thinkLevel, setThinkLevel] = useState<"fast" | "normal" | "deep">("normal");
  const [themeNow, setTheme] = useState<Theme>(getTheme);
  const [switchingModel, setSwitchingModel] = useState(false);

  const setPermLevel = useCallback((level: PermLevel) => {
    setPermLevelState(level);
    app.SetPermLevel(level).catch(() => {});
    // Plan mode on for all modes — model decides whether to research or act
    setPlan(true);
  }, [setPlan]);

  const handleThinkLevelChange = useCallback(async (level: string) => {
    setThinkLevel(level as "fast" | "normal" | "deep");
    const temp = THINK_TEMPS[level] ?? 0.3;
    try {
      const settings = await app.Settings();
      app.SetAgentParams(temp, settings.agent.maxSteps, settings.agent.systemPrompt).catch(() => {});
    } catch {
      app.SetAgentParams(temp, 0, "").catch(() => {});
    }
  }, []);

  const switchModel = useCallback(
    async (name: string) => {
      setSwitchingModel(true);
      await setModel(name);
      setSwitchingModel(false);
    },
    [setModel],
  );

  return { permLevel, setPermLevel, thinkLevel, themeNow, setTheme, switchingModel, handleThinkLevelChange, switchModel };
}
