import { useEffect, useState } from "react";
import { SettingsPageShell, SettingsField, SegmentedButton } from "./SettingsPageShell";
import { Plus, Trash2, Power, PowerOff } from "lucide-react";
import { app } from "../lib/bridge";
import type { PluginEntryView } from "../lib/types";

const TRANSPORT_TYPES = ["stdio", "http", "sse"] as const;

function emptyPlugin(): PluginEntryView {
  return { name: "", type: "stdio", command: "", args: [], env: {}, url: "", headers: {}, autoStart: null };
}

export function SettingsPlugins() {
  const [plugins, setPlugins] = useState<PluginEntryView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [editPlugin, setEditPlugin] = useState<PluginEntryView | null>(null);
  const [envText, setEnvText] = useState("");
  const [headersText, setHeadersText] = useState("");
  const [argsText, setArgsText] = useState("");

  const reload = async () => {
    try {
      const list: PluginEntryView[] = await (app as any).Plugins();
      setPlugins(list || []);
      setError(null);
    } catch (e: any) {
      setError(String(e));
      if (!plugins) setPlugins([]);
    }
  };

  useEffect(() => { reload(); }, []);

  const save = async (p: PluginEntryView) => {
    setBusy(true);
    try {
      await (app as any).SavePlugin(p);
      setEditPlugin(null);
      await reload();
    } catch (e: any) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (name: string) => {
    setBusy(true);
    try {
      await (app as any).RemovePlugin(name);
      await reload();
    } catch (e: any) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const toggleEnabled = async (name: string, enabled: boolean) => {
    setBusy(true);
    try {
      await (app as any).SetPluginEnabled(name, enabled);
      await reload();
    } catch (e: any) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const startEdit = (p?: PluginEntryView) => {
    const e = p ? { ...p, args: [...(p.args || [])], env: { ...(p.env || {}) }, headers: { ...(p.headers || {}) } } : emptyPlugin();
    setEditPlugin(e);
    setEnvText(mapToText(e.env));
    setHeadersText(mapToText(e.headers));
    setArgsText((e.args || []).join("\n"));
  };

  const commitEdit = () => {
    if (!editPlugin) return;
    const p = {
      ...editPlugin,
      env: textToMap(envText),
      headers: textToMap(headersText),
      args: argsText.split("\n").map(s => s.trim()).filter(Boolean),
    };
    save(p);
  };

  if (!plugins) {
    return (
      <SettingsPageShell title="插件" desc="管理 config.toml [[plugins]] 段中的 MCP 服务器。">
        <div className="text-fg-faint text-[13px] py-8 text-center">Loading...</div>
      </SettingsPageShell>
    );
  }

  return (
    <SettingsPageShell title="插件" desc="管理 config.toml [[plugins]] 段中的 MCP 服务器扩展。保存后自动重建控制器。">
      {error && (
        <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-3">
          {error}
        </div>
      )}
      {busy && (
        <div className="text-accent text-[12px] mb-3">Saving...</div>
      )}

      <div className="mb-3">
        <button
          className="flex items-center gap-1 text-[12px] bg-accent text-white border-0 rounded-md px-3 py-1.5 cursor-pointer"
          onClick={() => startEdit()}
        >
          <Plus size={13} /> Add Plugin
        </button>
      </div>

      {plugins.map(p => {
        const isEnabled = p.autoStart !== false;
        return (
          <div key={p.name} className="bg-bg-soft border border-border-soft rounded-lg p-3 mb-2">
            <div className="flex items-center justify-between">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className={"w-2 h-2 rounded-full " + (isEnabled ? "bg-green-400" : "bg-gray-500")} />
                  <span className="text-fg text-[13px] font-medium">{p.name}</span>
                  <span className="text-fg-faint text-[10px] bg-bg rounded px-1.5 py-0.5">{p.type || "stdio"}</span>
                  {!isEnabled && <span className="text-[10px] text-fg-faint">disabled</span>}
                </div>
                <span className="text-fg-faint text-[11px] block truncate mt-0.5">
                  {p.type === "http" || p.type === "sse" ? p.url : p.command || "no command"}
                </span>
              </div>
              <div className="flex items-center gap-1 ml-2">
                <button className="p-1 cursor-pointer bg-transparent border-0 rounded"
                  onClick={() => toggleEnabled(p.name, !isEnabled)}>
                  {isEnabled ? <PowerOff size={14} className="text-yellow-400" /> : <Power size={14} className="text-fg-faint" />}
                </button>
                <button className="px-2 py-0.5 text-[11px] text-fg-dim bg-transparent border border-border-soft rounded cursor-pointer hover:text-accent"
                  onClick={() => startEdit(p)}>Edit</button>
                <button className="p-1 text-fg-faint hover:text-red-400 cursor-pointer bg-transparent border-0 rounded"
                  onClick={() => { if (confirm("Delete plugin " + p.name + "?")) remove(p.name); }}>
                  <Trash2 size={13} />
                </button>
              </div>
            </div>
          </div>
        );
      })}

      {plugins.length === 0 && (
        <div className="text-fg-faint text-[12px] py-4 text-center">No plugins configured.</div>
      )}

      {editPlugin && (
        <div className="fixed inset-0 z-50 bg-black/60 flex items-center justify-center" onClick={() => setEditPlugin(null)}>
          <div className="bg-bg-soft border border-border-soft rounded-xl w-[520px] max-h-[85vh] overflow-y-auto p-5 shadow-2xl" onClick={e => e.stopPropagation()}>
            <h2 className="text-fg text-[15px] font-semibold mb-4">
              {editPlugin.name ? "Edit " + editPlugin.name : "Add Plugin"}
            </h2>

            <div className="space-y-3">
              <SettingsField label="Name" hint="Unique identifier">
                <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
                  value={editPlugin.name}
                  onChange={e => setEditPlugin({ ...editPlugin, name: e.target.value })}
                  placeholder="codegraph" />
              </SettingsField>

              <SettingsField label="Transport" hint="stdio=subprocess, http=remote URL">
                <SegmentedButton
                  options={TRANSPORT_TYPES.map(t => ({ value: t, label: t }))}
                  value={editPlugin.type || "stdio"}
                  onChange={v => setEditPlugin({ ...editPlugin, type: v })}
                />
              </SettingsField>

              {(editPlugin.type === "http" || editPlugin.type === "sse") ? (
                <SettingsField label="URL" hint="Remote MCP server URL">
                  <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
                    value={editPlugin.url || ""}
                    onChange={e => setEditPlugin({ ...editPlugin, url: e.target.value })}
                    placeholder="https://example.com/mcp" />
                </SettingsField>
              ) : (
                <SettingsField label="Command" hint="Local executable path">
                  <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
                    value={editPlugin.command || ""}
                    onChange={e => setEditPlugin({ ...editPlugin, command: e.target.value })}
                    placeholder="npx -y @anthropic/mcp-server" />
                </SettingsField>
              )}

              <SettingsField label="Args" hint="One per line">
                <textarea className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[50px]"
                  value={argsText}
                  onChange={e => setArgsText(e.target.value)}
                  rows={3} />
              </SettingsField>

              <SettingsField label="Env" hint="KEY=VALUE per line">
                <textarea className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[50px]"
                  value={envText}
                  onChange={e => setEnvText(e.target.value)}
                  rows={3} />
              </SettingsField>

              <SettingsField label="Headers" hint="Header: Value per line">
                <textarea className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[50px]"
                  value={headersText}
                  onChange={e => setHeadersText(e.target.value)}
                  placeholder="Authorization: Bearer xxx"
                  rows={3} />
              </SettingsField>

              <SettingsField label="Auto-start" hint="Connect on session startup">
                <SegmentedButton
                  options={[{ value: "true", label: "Yes" }, { value: "false", label: "No" }]}
                  value={editPlugin.autoStart === false ? "false" : "true"}
                  onChange={v => setEditPlugin({ ...editPlugin, autoStart: v === "true" ? null : false })}
                />
              </SettingsField>
            </div>

            <div className="flex justify-end gap-2 mt-4 pt-3 border-t border-border-soft">
              <button className="px-3 py-1.5 text-[12px] bg-transparent border border-border-soft rounded text-fg-dim cursor-pointer"
                onClick={() => setEditPlugin(null)}>Cancel</button>
              <button className="px-3 py-1.5 text-[12px] bg-accent text-white border-0 rounded cursor-pointer"
                onClick={commitEdit}>
                {editPlugin.name ? "Save" : "Add"}
              </button>
            </div>
          </div>
        </div>
      )}
    </SettingsPageShell>
  );
}

function mapToText(m: Record<string, string>): string {
  if (!m) return "";
  return Object.entries(m).map(([k, v]) => k + "=" + v).join("\n");
}

function textToMap(t: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of t.split("\n")) {
    const eq = line.indexOf("=");
    if (eq > 0) {
      const k = line.slice(0, eq).trim();
      const v = line.slice(eq + 1).trim();
      if (k) out[k] = v;
    }
  }
  return out;
}
