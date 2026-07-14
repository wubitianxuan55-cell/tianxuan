import { useEffect, useState } from "react";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";
import { app } from "../lib/bridge";

interface CodegraphSettingsView { enabled: boolean; autoInstall: boolean; path: string; }

export function SettingsCodegraph() {
  const [v, setV] = useState<CodegraphSettingsView | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    (app as any).CodegraphSettings().then(setV).catch((e: any) => setErr(String(e)));
  }, []);

  const save = async (next: CodegraphSettingsView) => {
    setV(next);
    try { await (app as any).SaveCodegraphSettings(next); } catch (e: any) { setErr(String(e)); }
  };

  if (!v) return <SettingsPageShell title="Codegraph" desc="Code knowledge graph configuration."><div className="text-fg-faint py-8 text-center">Loading...</div></SettingsPageShell>;

  return (
    <SettingsPageShell title="Codegraph" desc="Code knowledge graph for symbol search and relationship analysis.">
      {err && <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-3">{err}</div>}
      <SettingsSection title="Codegraph">
        <SettingsField label="Enable" hint="Index code symbols and relationships">
          <label className="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" checked={v.enabled} onChange={e => save({ ...v, enabled: e.target.checked })} className="w-4 h-4 accent-accent" />
            <span className="text-fg text-[13px]">Enabled</span>
          </label>
        </SettingsField>
        <SettingsField label="Auto-install" hint="Auto-install gitnexus binary if missing">
          <label className="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" checked={v.autoInstall} onChange={e => save({ ...v, autoInstall: e.target.checked })} className="w-4 h-4 accent-accent" />
            <span className="text-fg text-[13px]">Auto-install</span>
          </label>
        </SettingsField>
        <SettingsField label="Binary path" hint="Custom gitnexus binary path (empty=auto)">
          <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            value={v.path} onChange={e => save({ ...v, path: e.target.value })} placeholder="auto-detect" />
        </SettingsField>
      </SettingsSection>
    </SettingsPageShell>
  );
}
