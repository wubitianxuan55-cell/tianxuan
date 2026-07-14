import { useEffect, useState } from "react";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";
import { app } from "../lib/bridge";

interface SearchSettingsView {
  localSearXNGUrl: string;
  tavilyApiKeyEnv: string;
  braveApiKeyEnv: string;
  timeoutSeconds: number;
  allowDomains: string[];
  denyDomains: string[];
}

export function SettingsSearch() {
  const [v, setV] = useState<SearchSettingsView | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    (app as any).SearchSettings().then(setV).catch((e: any) => setErr(String(e)));
  }, []);

  const save = async (next: SearchSettingsView) => {
    setV(next);
    try { await (app as any).SaveSearchSettings(next); setErr(null); } catch (e: any) { setErr(String(e)); }
  };

  if (!v) return <SettingsPageShell title="Search" desc="Web search engine configuration."><div className="text-fg-faint py-8 text-center">Loading...</div></SettingsPageShell>;

  return (
    <SettingsPageShell title="Search" desc="Configure SearXNG, Tavily, and Brave search engines.">
      {err && <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-3">{err}</div>}
      <SettingsSection title="Engines">
        <SettingsField label="SearXNG URL" hint="Self-hosted SearXNG instance URL">
          <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            value={v.localSearXNGUrl} onChange={e => save({ ...v, localSearXNGUrl: e.target.value })} placeholder="http://localhost:8080" />
        </SettingsField>
        <SettingsField label="Tavily API Key Env" hint="Env var name for Tavily API key">
          <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            value={v.tavilyApiKeyEnv} onChange={e => save({ ...v, tavilyApiKeyEnv: e.target.value })} placeholder="TAVILY_API_KEY" />
        </SettingsField>
        <SettingsField label="Brave API Key Env" hint="Env var name for Brave Search API key">
          <input className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            value={v.braveApiKeyEnv} onChange={e => save({ ...v, braveApiKeyEnv: e.target.value })} placeholder="BRAVE_API_KEY" />
        </SettingsField>
        <SettingsField label="Timeout (s)" hint="Per-engine HTTP timeout, 0=default(10s)">
          <input type="number" min="0" max="120" className="w-24 bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            value={v.timeoutSeconds || ""} onChange={e => save({ ...v, timeoutSeconds: Number(e.target.value) || 0 })} />
        </SettingsField>
      </SettingsSection>
      <SettingsSection title="Domain Filter">
        <SettingsField label="Allow" hint="One per line, supports *.example.com">
          <textarea className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[60px]"
            value={(v.allowDomains||[]).join("\n")}
            onChange={e => save({ ...v, allowDomains: e.target.value.split("\n").map(s => s.trim()).filter(Boolean) })} rows={3} />
        </SettingsField>
        <SettingsField label="Deny" hint="One per line, supports *.example.com">
          <textarea className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[60px]"
            value={(v.denyDomains||[]).join("\n")}
            onChange={e => save({ ...v, denyDomains: e.target.value.split("\n").map(s => s.trim()).filter(Boolean) })} rows={3} />
        </SettingsField>
      </SettingsSection>
    </SettingsPageShell>
  );
}
