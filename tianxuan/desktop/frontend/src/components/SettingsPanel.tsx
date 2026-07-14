import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { applyColorScheme, applyThemeMode, getColorScheme, getThemeMode, type ColorScheme, type ThemeMode } from "../lib/theme";
import type { SettingsView } from "../lib/types";
import { CloseButton } from "./CloseButton";
import { Modal } from "./Modal";
import { Cpu, Shield, Box, Bot, Palette, CloudUpload, Plug, Cog, Globe, Wrench, Puzzle, Braces, Zap, BrainCircuit, Command, Search } from "lucide-react";
import { ModelsSection } from "./SettingsModels";
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
import { SettingsSubagents } from "./SettingsSubagents";
import { SettingsPlugins } from "./SettingsPlugins";
import { SettingsHooks } from "./SettingsHooks";
import { SettingsShortcuts } from "./SettingsShortcuts";
import { SettingsDiagnostics } from "./SettingsDiagnostics";
import { SETTINGS_TABS, TAB_GROUPS, settingsTabLabel, settingsTabMeta, type SettingsTab } from "./SettingsShared";

type TabRenderers = Record<SettingsTab, () => React.ReactNode>;

export function SettingsPanel({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  const t = useT();
  const [s, setS] = useState<SettingsView | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [scheme, setSchemeState] = useState<ColorScheme>(getColorScheme());
  const [mode, setModeState] = useState<ThemeMode>(getThemeMode());
  const [tab, setTab] = useState<SettingsTab>("general");
  const [query, setQuery] = useState("");

  const TAB_ICONS: Record<SettingsTab, React.ReactNode> = {
    general: <Cog size={14} />,
    models: <Cpu size={14} />,
    providers: <Plug size={14} />,
    permissions: <Shield size={14} />,
    sandbox: <Box size={14} />,
    agent: <Bot size={14} />,
    network: <Globe size={14} />,
    appearance: <Palette size={14} />,
    updates: <CloudUpload size={14} />,
    shortcuts: <Command size={14} />,
    mcp: <Wrench size={14} />,
    skills: <Zap size={14} />,
    subagents: <Braces size={14} />,
    plugins: <Puzzle size={14} />,
    memory: <BrainCircuit size={14} />,
    hooks: <Zap size={14} />,
    diagnostics: <Search size={14} />,
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
    models: () => <ModelsSection s={s} busy={busy} apply={apply} onManageProviders={() => setTab("providers")} />,
    providers: () => <ProvidersSection s={s} busy={busy} apply={apply} />,
    permissions: () => <PermissionsSection s={s} busy={busy} apply={apply} />,
    sandbox: () => <SandboxSection s={s} busy={busy} apply={apply} />,
    agent: () => <AgentSection s={s} busy={busy} apply={apply} />,
    network: () => <SettingsNetwork s={s} busy={busy} apply={apply} />,
    appearance: () => (
      <AppearanceSection scheme={scheme} mode={mode}
        onScheme={(sc) => { applyColorScheme(sc); setSchemeState(sc); }}
        onMode={(m) => { applyThemeMode(m); setModeState(m); }} />
    ),
    updates: () => <UpdatesSection configPath={s.configPath} />,
    shortcuts: () => <SettingsShortcuts />,
    mcp: () => <SettingsMcp />,
    skills: () => <SettingsSkills />,
    subagents: () => <SettingsSubagents s={s} busy={busy} apply={apply} />,
    plugins: () => <SettingsPlugins />,
    memory: () => <SettingsMemory />,
    hooks: () => <SettingsHooks />,
    diagnostics: () => <SettingsDiagnostics />,
  } : null;

  const visibleTabs = new Set(filteredTabs);
  const visibleGroups = TAB_GROUPS.map((g) => ({
    ...g, tabs: g.tabs.filter((t) => visibleTabs.has(t)),
  })).filter((g) => g.tabs.length > 0);

  return (
    <Modal onClose={onClose} wide>
      {/* 标题栏 */}
      <header className="flex items-center justify-between shrink-0 px-5 py-3.5 border-b border-border-soft">
        <span className="text-[15px] font-semibold text-fg">{t("settings.title")}</span>
        <CloseButton onClick={onClose} />
      </header>
      {!s ? (
        <div className="flex-1 flex items-center justify-center text-fg-faint text-[13px]">{t("settings.loading")}</div>
      ) : (
        <div className="flex-1 min-h-0 flex overflow-y-auto"><div className="flex h-full">
          <nav className="flex flex-col gap-1 w-[220px] py-2.5 px-2 border-r border-border-soft overflow-y-auto shrink-0">
            <div className="relative mb-2">
              <input className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] pl-7 pr-2 py-1.5 outline-none placeholder:text-fg-faint/50 focus:border-accent transition-colors"
                placeholder="搜索…" value={query} onChange={(e) => setQuery(e.target.value)} />
              <svg className="absolute left-2 top-1/2 -translate-y-1/2 text-fg-faint/40" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
            </div>
            {query.trim() ? (
              filteredTabs.length === 0
                ? <div className="px-3 py-4 text-center text-[11px] text-fg-faint">无匹配</div>
                : filteredTabs.map((id) => <NavButton key={id} id={id} s={s} t={t} active={tab===id} icons={TAB_ICONS} onClick={()=>{setTab(id);setQuery("")}} />)
            ) : (
              visibleGroups.map((group) => (
                <div key={group.label} className="mb-2">
                  <div className="px-3 py-1 text-[10.5px] font-semibold uppercase tracking-widest text-fg-faint/50">{group.label}</div>
                  {group.tabs.map((id) => <NavButton key={id} id={id} s={s} t={t} active={tab===id} icons={TAB_ICONS} onClick={()=>setTab(id)} />)}
                </div>
              ))
            )}
          </nav>
          <main className="flex-1 min-w-0 overflow-y-auto px-6 py-4">
            {err && <div className="shrink-0 px-4 py-2 mb-3 text-[12.5px] bg-del-bg text-err rounded-md border border-err/20">{err}</div>}
            {renderers?.[tab]?.()}
          </main>
        </div></div>
      )}
    </Modal>
  );
}
function NavButton({ id, s, t, active, icons, onClick }: {
  id: SettingsTab; s: SettingsView; t: ReturnType<typeof useT>; active: boolean;
  icons: Record<SettingsTab, React.ReactNode>; onClick: () => void;
}) {
  return (
    <button className={`flex items-center gap-2.5 w-full px-3 py-2 border-0 rounded-lg bg-transparent text-left cursor-pointer transition-[color,background] duration-[var(--dur-fast)] ${active?"text-accent bg-accent-soft":"text-fg-dim hover:text-fg hover:bg-bg-soft"}`}
      onClick={onClick}>
      <span className="shrink-0 opacity-70">{icons[id]}</span>
      <div className="flex flex-col gap-0.5 min-w-0">
        <span className="text-[13px] font-medium">{settingsTabLabel(id, t)}</span>
        <small className="text-[11px] text-fg-faint truncate">{settingsTabMeta(id, s, t)}</small>
      </div>
    </button>
  );
}
