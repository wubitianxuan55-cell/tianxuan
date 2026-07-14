import { useEffect, useState } from "react";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";
import { app } from "../lib/bridge";
import type { LSPSettingsView } from "../lib/types";

export function SettingsLsp() {
  const [v, setV] = useState<LSPSettingsView | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    app.LSPSettings().then(setV).catch((e: any) => setErr(String(e)));
  }, []);

  const save = async (next: LSPSettingsView) => {
    setV(next);
    try { await app.SaveLSPSettings(next); } catch (e: any) { setErr(String(e)); }
  };

  if (!v) return <SettingsPageShell title="LSP" desc="Language Server Protocol configuration."><div className="text-fg-faint py-8 text-center">Loading...</div></SettingsPageShell>;

  const count = Object.keys(v.servers||{}).length;

  return (
    <SettingsPageShell title="LSP" desc="Configure language servers for symbol navigation, diagnostics, and hover.">
      {err && <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-3">{err}</div>}
      <SettingsSection title="LSP">
        <SettingsField label="Enable LSP" hint="Enable language server integration">
          <label className="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" checked={v.enabled} onChange={e => save({ ...v, enabled: e.target.checked })}
              className="w-4 h-4 accent-accent" />
            <span className="text-fg text-[13px]">Enabled</span>
          </label>
        </SettingsField>
        <div className="mt-3 text-fg-faint text-[11px]">
          {count} language server(s) configured in config.toml [lsp.servers].
        </div>
      </SettingsSection>
    </SettingsPageShell>
  );
}
