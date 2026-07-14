import { MemoryPanelContent } from "./MemoryPanelContent";
import { SettingsPageShell } from "./SettingsPageShell";
import { useT } from "../lib/i18n";

export function SettingsMemory() {
  const t = useT();
  return (
    <SettingsPageShell title={t("settings.tab.memory")} desc={t("memory.summary", { facts: 0, docs: 0 })}>
      <MemoryPanelContent />
    </SettingsPageShell>
  );
}
