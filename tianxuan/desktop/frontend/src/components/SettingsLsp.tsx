import { useEffect, useState } from "react";
import { Code2 } from "lucide-react";
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

  if (!v) return <SettingsPageShell title={<span className="flex items-center gap-1.5"><Code2 size={15} />LSP</span>} desc="Language Server Protocol 配置。"><div className="text-fg-faint py-8 text-center">Loading...</div></SettingsPageShell>;

  const count = Object.keys(v.servers||{}).length;

  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><Code2 size={15} />LSP</span>} desc="配置语言服务器，用于符号导航、诊断和悬停提示。">
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
