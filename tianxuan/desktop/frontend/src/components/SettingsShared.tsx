import type { SettingsView } from "../lib/types";
import { useT } from "../lib/i18n";

export type SettingsTab =
  | "general" | "providers" | "agent" | "permissions" | "sandbox"
  | "network" | "appearance" | "updates" | "shortcuts"
  | "search" | "lsp" | "codegraph"
  | "mcp" | "skills" | "memory" | "hooks";

export const SETTINGS_TABS: SettingsTab[] = [
  "general", "providers", "agent", "permissions", "sandbox",
  "network", "appearance", "updates", "shortcuts",
  "mcp", "skills", "memory", "hooks",
];

export type TabGroup = { label: string; tabs: SettingsTab[] };

export const TAB_GROUPS: TabGroup[] = [
  { label: "核心", tabs: ["general", "providers", "agent", "permissions", "sandbox"] },
  { label: "环境", tabs: ["network", "appearance", "updates", "shortcuts"] },
  { label: "能力", tabs: ["mcp", "skills", "memory", "hooks"] },
];

export type SectionProps = {
  s: SettingsView;
  busy: boolean;
  apply: (fn: () => Promise<void>) => Promise<void>;
};

export function settingsTabLabel(id: SettingsTab, t: ReturnType<typeof useT>): string {
  const fallback: Record<string, string> = {
    general: "通用", providers: "模型服务", agent: "智能体", permissions: "权限",
    sandbox: "沙箱", network: "网络", appearance: "外观",
    updates: "更新", shortcuts: "快捷键",
    search: "搜索", lsp: "LSP", codegraph: "Codegraph",
    mcp: "MCP", skills: "技能", memory: "记忆", hooks: "钩子",
  };
  try { return t(`settings.tab.${id}` as any); }
  catch { return fallback[id] || id; }
}

export function settingsTabMeta(id: SettingsTab, s: SettingsView, t: ReturnType<typeof useT>): string {
  switch (id) {
    case "general": return t("settings.generalMeta");
    case "agent": return t("settings.agentMeta", { temp: s.agent.temperature, depth: s.agent.maxSubagentDepth || 0, steps: s.agent.maxSteps || "∞" });
    case "providers": return t("settings.providerCount", { n: s.providers.length });
    case "permissions": return s.permissions.mode;
    case "sandbox": return s.sandbox.bash;
    case "network": return s.network?.proxyMode || "off";
    case "appearance": return t("settings.appearanceMeta");
    case "updates": return t("settings.updatesMeta");
    case "shortcuts": return t("settings.shortcutsMeta");
    case "mcp": return t("settings.mcpMeta");
    case "skills": return t("settings.skillsMeta");
    case "memory": return t("settings.memoryMeta");
    case "hooks": return t("settings.hooksMeta");
    case "search": return "search";
    case "lsp": return "lsp";
    case "codegraph": return "codegraph";
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
