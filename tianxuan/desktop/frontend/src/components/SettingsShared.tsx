import type { SettingsView } from "../lib/types";
import { useT } from "../lib/i18n";

export type SettingsTab =
  | "general" | "models" | "providers" | "permissions" | "sandbox" | "agent"
  | "network" | "appearance" | "updates" | "shortcuts"
  | "mcp" | "skills" | "subagents" | "plugins" | "memory" | "hooks";

export const SETTINGS_TABS: SettingsTab[] = [
  "general", "models", "providers", "permissions", "sandbox", "agent",
  "network", "appearance", "updates", "shortcuts",
  "mcp", "skills", "subagents", "plugins", "memory", "hooks",
];

export type TabGroup = { label: string; tabs: SettingsTab[] };

export const TAB_GROUPS: TabGroup[] = [
  { label: "核心", tabs: ["general", "models", "providers", "permissions", "sandbox", "agent"] },
  { label: "环境", tabs: ["network", "appearance", "updates", "shortcuts"] },
  { label: "能力", tabs: ["mcp", "skills", "subagents", "plugins", "memory", "hooks"] },
];

export type SectionProps = {
  s: SettingsView;
  busy: boolean;
  apply: (fn: () => Promise<void>) => Promise<void>;
};

export function settingsTabLabel(id: SettingsTab, t: ReturnType<typeof useT>): string {
  const fallback: Record<string, string> = {
    general: "通用", models: "模型", providers: "模型服务", permissions: "权限",
    sandbox: "沙箱", agent: "智能体", network: "网络", appearance: "外观",
    updates: "更新", shortcuts: "快捷键",
    mcp: "MCP", skills: "技能", subagents: "子代理", plugins: "插件",
    memory: "记忆", hooks: "钩子",
  };
  try { return t(`settings.tab.${id}` as any); }
  catch { return fallback[id] || id; }
}

export function settingsTabMeta(id: SettingsTab, s: SettingsView, t: ReturnType<typeof useT>): string {
  switch (id) {
    case "general": return `${s.agent.autoPlan || "off"} · ${s.agent.reasoningLanguage || "auto"}`;
    case "models": return toRef(s.defaultModel, s) || t("common.none");
    case "providers": return t("settings.providerCount", { n: s.providers.length });
    case "permissions": return s.permissions.mode;
    case "sandbox": return s.sandbox.bash;
    case "agent": return t("settings.agentMeta", { temp: s.agent.temperature, depth: s.agent.maxSubagentDepth || 0, steps: s.agent.maxSteps || "∞" });
    case "network": return s.network?.proxyMode || "off";
    case "appearance": return t("settings.appearanceMeta");
    case "updates": return t("settings.updatesMeta");
    case "shortcuts": return "12 个";
    case "mcp": return "服务";
    case "skills": return "项目";
    case "subagents": return "配置";
    case "plugins": return "扩展";
    case "memory": return "记忆";
    case "hooks": return "事件";
  }
}

export function allRefs(s: SettingsView): string[] {
  const out: string[] = [];
  for (const p of s.providers) for (const m of p.models) out.push(`${p.name}/${m}`);
  return out;
}

export function toRef(model: string, s: SettingsView): string {
  if (!model) return "";
  if (model.includes("/")) return model;
  const byName = s.providers.find((p) => p.name === model);
  if (byName) return `${byName.name}/${byName.default || byName.models[0] || ""}`;
  const byModel = s.providers.find((p) => p.models.includes(model));
  if (byModel) return `${byModel.name}/${model}`;
  return model;
}
