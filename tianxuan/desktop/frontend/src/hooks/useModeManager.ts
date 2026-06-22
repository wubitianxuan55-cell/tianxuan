// 模式管理 hook — V9.0 统一模式：explore/develop/orchestrate + YOLO toggle + thinkLevel
import { useState, useCallback } from "react";
import type { AgentMode } from "../lib/types";
import { getTheme } from "../lib/theme";
import type { Theme } from "../lib/theme";
import { app } from "../lib/bridge";

const THINK_TEMPS: Record<string, number> = { fast: 0.1, normal: 0.3, deep: 0.7 };

export function useModeManager(
  setPlan: (on: boolean) => void,
  setBypass: (on: boolean) => void,
  setModel: (name: string) => Promise<void>,
) {
  const [agentMode, setAgentModeState] = useState<AgentMode>("develop");
  const [yolo, setYoloState] = useState(false);
  const [thinkLevel, setThinkLevel] = useState<"fast" | "normal" | "deep">("normal");
  const [themeNow, setTheme] = useState<Theme>(getTheme);
  const [switchingModel, setSwitchingModel] = useState(false);

  const setAgentMode = useCallback((m: AgentMode) => {
    setAgentModeState(m);
    app.SetAgentMode(m).catch(() => {});
    // Sync plan flag: explore/orchestrate → plan mode; develop → full tools
    if (m === "explore" || m === "orchestrate") setPlan(true);
    else setPlan(false);
    // YOLO only available in develop/orchestrate; turn off if switching to explore
    if (m === "explore") {
      setYoloState(false);
      app.SetBypass(false).catch(() => {});
    }
  }, [setPlan]);

  const toggleYolo = useCallback(() => {
    setYoloState(prev => {
      const next = !prev;
      app.SetBypass(next).catch(() => {});
      setBypass(next);
      return next;
    });
  }, [setBypass]);

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

  return { agentMode, setAgentMode, yolo, toggleYolo, thinkLevel, themeNow, setTheme, switchingModel, handleThinkLevelChange, switchModel };
}
