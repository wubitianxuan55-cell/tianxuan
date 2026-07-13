import type { SettingsView } from "../lib/types";
import { useT } from "../lib/i18n";

export type SettingsTab = "models" | "providers" | "permissions" | "sandbox" | "agent" | "appearance" | "updates";

export const SETTINGS_TABS: SettingsTab[] = ["models", "providers", "permissions", "sandbox", "agent", "appearance", "updates"];

export type SectionProps = {
  s: SettingsView;
  busy: boolean;
  apply: (fn: () => Promise<void>) => Promise<void>;
};

export function settingsTabLabel(id: SettingsTab, t: ReturnType<typeof useT>): string {
  switch (id) {
    case "models":
      return t("settings.tab.models");
    case "providers":
      return t("settings.tab.providers");
    case "permissions":
      return t("settings.tab.permissions");
    case "sandbox":
      return t("settings.tab.sandbox");
    case "agent":
      return t("settings.tab.agent");
    case "appearance":
      return t("settings.tab.appearance");
    case "updates":
      return t("settings.tab.updates");
  }
}

export function settingsTabMeta(id: SettingsTab, s: SettingsView, t: ReturnType<typeof useT>): string {
  switch (id) {
    case "models":
      return toRef(s.defaultModel, s) || t("common.none");
    case "providers":
      return t("settings.providerCount", { n: s.providers.length });
    case "permissions":
      return s.permissions.mode;
    case "sandbox":
      return s.sandbox.bash;
    case "agent":
      return t("settings.agentMeta", { temp: s.agent.temperature, steps: s.agent.maxSteps || "∞" });
    case "appearance":
      return t("settings.appearanceMeta");
    case "updates":
      return t("settings.updatesMeta");
  }
}

// allRefs flattens providers into "provider/model" refs for the model selectors.
export function allRefs(s: SettingsView): string[] {
  const out: string[] = [];
  for (const p of s.providers) for (const m of p.models) out.push(`${p.name}/${m}`);
  return out;
}

// toRef normalises a stored model id (a provider name, a bare model, or a ref) to
// a "provider/model" ref so a <select> of refs can show it selected.
export function toRef(model: string, s: SettingsView): string {
  if (!model) return "";
  if (model.includes("/")) return model;
  const byName = s.providers.find((p) => p.name === model);
  if (byName) return `${byName.name}/${byName.default || byName.models[0] || ""}`;
  const byModel = s.providers.find((p) => p.models.includes(model));
  if (byModel) return `${byModel.name}/${model}`;
  return model;
}
