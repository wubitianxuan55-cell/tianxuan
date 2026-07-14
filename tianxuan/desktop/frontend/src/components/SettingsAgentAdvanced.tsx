import { useEffect, useState } from "react";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";
import { app } from "../lib/bridge";
import type { AgentAdvancedView } from "../lib/types";

export function SettingsAgentAdvanced() {
  const [v, setV] = useState<AgentAdvancedView | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    app.AgentAdvancedSettings().then(setV).catch((e: any) => setErr(String(e)));
  }, []);

  const save = async (next: AgentAdvancedView) => {
    setV(next);
    try { await app.SaveAgentAdvancedSettings(next); } catch (e: any) { setErr(String(e)); }
  };

  if (!v) return <SettingsPageShell title="Agent Advanced" desc="Advanced agent runtime parameters."><div className="text-fg-faint py-8 text-center">Loading...</div></SettingsPageShell>;

  return (
    <SettingsPageShell title="Agent Advanced" desc="Advanced agent parameters in config.toml [agent].">
      {err && <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-3">{err}</div>}
      <SettingsSection title="Advanced">
        <SettingsField label="System Prompt File" hint="Path to a file whose contents replace the system prompt">
          <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            value={v.systemPromptFile} onChange={e => save({ ...v, systemPromptFile: e.target.value })} placeholder="system_prompt.md" />
        </SettingsField>
        <SettingsField label="Auto-plan Classifier" hint="Model used for borderline auto-plan decisions (empty=heuristic)">
          <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            value={v.autoPlanClassifier} onChange={e => save({ ...v, autoPlanClassifier: e.target.value })}
            placeholder="provider/model (e.g. deepseek/deepseek-v4-flash)" />
        </SettingsField>
      </SettingsSection>
    </SettingsPageShell>
  );
}
