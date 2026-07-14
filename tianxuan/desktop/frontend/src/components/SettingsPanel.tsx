import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { SettingsView } from "../lib/types";
import { Search } from "lucide-react";
import { CloseButton } from "./CloseButton";
import { Modal } from "./Modal";
import { Shield, Box, Bot, Palette, CloudUpload, Plug, Cog, Globe, Wrench, Zap, BrainCircuit, Command, Code2, Monitor } from "lucide-react";
import { ProvidersSection } from "./SettingsProviders";
import { PermissionsSection } from "./SettingsPermissions";
import { SandboxSection } from "./SettingsSandbox";
import { AgentSection } from "./SettingsAgent";
import { AppearanceSection } from "./SettingsAppearance";
import { UpdatesSection } from "./SettingsUpdates";
import { SettingsGeneral } from "./SettingsGeneral";
import { SettingsNetwork } from "./SettingsNetwork";
import { SettingsMcp } from "./SettingsMcp";
import { SettingsSkills } from "./SettingsSkills";
import { SettingsMemory } from "./SettingsMemory";
import { SettingsHooks } from "./SettingsHooks";
import { SettingsShortcuts } from "./SettingsShortcuts";
import { SettingsSearch } from "./SettingsSearch";
import { SettingsLsp } from "./SettingsLsp";
import { SettingsCodegraph } from "./SettingsCodegraph";
import { SETTINGS_TABS, TAB_GROUPS, settingsTabLabel, settingsTabMeta, type SettingsTab } from "./SettingsShared";

type TabRenderers = Record<SettingsTab, () => React.ReactNode>;

export function SettingsPanel({ onClose, onChanged, initialTab }: { onClose: () => void; onChanged: () => void; initialTab?: SettingsTab }) {
  const t = useT();
  const [s, setS] = useState<SettingsView | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [tab, setTab] = useState<SettingsTab>(initialTab || "general");
  const [query, setQuery] = useState("");

  // reset to initialTab when the panel reopens with a new target
  useEffect(() => { if (initialTab) setTab(initialTab); }, [initialTab]);

  const TAB_ICONS: Record<SettingsTab, React.ReactNode> = {
    general: <Cog size={15} />,
providers: <Plug size={15} />,
    permissions: <Shield size={15} />,
    sandbox: <Box size={15} />,
    agent: <Bot size={15} />,
    network: <Globe size={15} />,
    appearance: <Palette size={15} />,
    updates: <CloudUpload size={15} />,
    shortcuts: <Command size={15} />,
    mcp: <Wrench size={15} />,
    skills: <Zap size={15} />,
    memory: <BrainCircuit size={15} />,
    hooks: <Zap size={15} />,
search: <Search size={15} />,
    lsp: <Code2 size={15} />,
    codegraph: <Monitor size={15} />,
  };

  const filteredTabs = query.trim() && s
    ? SETTINGS_TABS.filter((id) => {
        const label = settingsTabLabel(id, t).toLowerCase();
        const meta = settingsTabMeta(id, s, t).toLowerCase();
        return label.includes(query.toLowerCase()) || meta.includes(query.toLowerCase());
      })
    : SETTINGS_TABS;

  const reload = async () => setS(await app.Settings().catch(() => null));
  useEffect(() => { void reload(); }, []);

  const apply = async (fn: () => Promise<void>) => {
    setBusy(true); setErr(null);
    try { await fn(); await reload(); onChanged(); }
    catch (e) { setErr(String((e as Error)?.message ?? e)); }
    finally { setBusy(false); }
  };

  const renderers: TabRenderers | null = s ? {
    general: () => <SettingsGeneral s={s} busy={busy} apply={apply} />,
    providers: () => <ProvidersSection s={s} busy={busy} apply={apply} />,
    permissions: () => <PermissionsSection s={s} busy={busy} apply={apply} />,
    sandbox: () => <SandboxSection s={s} busy={busy} apply={apply} />,
    agent: () => <AgentSection s={s} busy={busy} apply={apply} />,
    network: () => <SettingsNetwork s={s} busy={busy} apply={apply} />,
    appearance: () => <AppearanceSection s={s} busy={busy} apply={apply} />,
    updates: () => <UpdatesSection configPath={s.configPath} />,
    shortcuts: () => <SettingsShortcuts />,
    mcp: () => <SettingsMcp />,
    skills: () => <SettingsSkills />,
    memory: () => <SettingsMemory />,
    hooks: () => <SettingsHooks />,
    search: () => <SettingsSearch />,
    lsp: () => <SettingsLsp />,
    codegraph: () => <SettingsCodegraph />,
  } : null;

  const visibleTabs = new Set(filteredTabs);
  const visibleGroups = TAB_GROUPS.map((g) => ({
    ...g, tabs: g.tabs.filter((t) => visibleTabs.has(t)),
  })).filter((g) => g.tabs.length > 0);

  return (
    <Modal onClose={onClose} wide>
      <header className="flex items-center justify-between shrink-0 px-5 py-3.5 border-b border-border-soft">
        <span className="text-[15px] font-semibold text-fg">{t("settings.title")}</span>
        <CloseButton onClick={onClose} />
      </header>
      {!s ? (
        <div className="flex-1 flex items-center justify-center text-fg-faint text-[13px]">{t("settings.loading")}</div>
      ) : (
        <div className="flex-1 min-h-0 flex">
          <nav className="flex flex-col gap-0.5 w-[224px] py-3 px-2 border-r border-border-soft overflow-y-auto shrink-0 scrollbar-thin">
            <div className="relative mb-2.5 px-0.5">
              <input className="w-full bg-bg border border-border rounded-md text-fg text-[12px] pl-7 pr-2.5 py-1.5 outline-none placeholder:text-fg-faint/40 focus:border-accent transition-colors"
                placeholder="搜索…" value={query} onChange={(e) => setQuery(e.target.value)} />
              <Search size={13} className="absolute left-2 top-1/2 -translate-y-1/2 text-fg-faint/40" />
            </div>
            {query.trim() ? (
              filteredTabs.length === 0
                ? <div className="px-3 py-4 text-center text-[11px] text-fg-faint">无匹配</div>
                : filteredTabs.map((id) => <NavButton key={id} id={id} s={s} t={t} active={tab===id} icons={TAB_ICONS} onClick={()=>{setTab(id);setQuery("")}} />)
            ) : (
              visibleGroups.map((group) => (
                <div key={group.label} className="mb-2">
                  <div className="px-3 pt-2 pb-1 text-[10.5px] font-semibold uppercase tracking-widest text-fg-faint/45">{group.label}</div>
                  {group.tabs.map((id) => <NavButton key={id} id={id} s={s} t={t} active={tab===id} icons={TAB_ICONS} onClick={()=>setTab(id)} />)}
                </div>
              ))
            )}
          </nav>
          <main className="flex-1 min-w-0 overflow-y-auto px-6 py-4">
            {err && <div className="shrink-0 px-4 py-2 mb-3 text-[12.5px] bg-del-bg text-err rounded-md border border-err/20">{err}</div>}
            {renderers?.[tab]?.()}
          </main>
        </div>
      )}
    </Modal>
  );
}
function NavButton({ id, s, t, active, icons, onClick }: {
  id: SettingsTab; s: SettingsView; t: ReturnType<typeof useT>; active: boolean;
  icons: Record<SettingsTab, React.ReactNode>; onClick: () => void;
}) {
  return (
    <button className={
      "relative flex items-center gap-2.5 w-full px-2.5 py-1.5 border-0 rounded-md bg-transparent text-left cursor-pointer transition-all duration-[var(--dur-fast)] group " +
      (active
        ? "text-accent before:absolute before:left-0 before:top-1.5 before:bottom-1.5 before:w-[3px] before:rounded-full before:bg-accent"
        : "text-fg-dim hover:text-fg hover:bg-sidebar-hover"
      )
    }
      onClick={onClick}>
      <span className="shrink-0 opacity-70 group-hover:opacity-90 transition-opacity">{icons[id]}</span>
      <div className="flex flex-col gap-0.5 min-w-0">
        <span className="text-[13px] font-medium">{settingsTabLabel(id, t)}</span>
        <small className="text-[11px] text-fg-faint truncate">{settingsTabMeta(id, s, t)}</small>
      </div>
    </button>
  );
}
