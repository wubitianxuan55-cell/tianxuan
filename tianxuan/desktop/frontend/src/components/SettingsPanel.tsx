import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { applyColorScheme, applyThemeMode, getColorScheme, getThemeMode, type ColorScheme, type ThemeMode } from "../lib/theme";
import type { SettingsView } from "../lib/types";
import { DrawerHeader, DrawerTitle } from "./DrawerHeader";
import { ResizableDrawer } from "./ResizableDrawer";
import { Cpu, Shield, Box, Bot, Palette, CloudUpload, Plug } from "lucide-react";
import { ModelsSection } from "./SettingsModels";
import { ProvidersSection } from "./SettingsProviders";
import { PermissionsSection } from "./SettingsPermissions";
import { SandboxSection } from "./SettingsSandbox";
import { AgentSection } from "./SettingsAgent";
import { AppearanceSection } from "./SettingsAppearance";
import { UpdatesSection } from "./SettingsUpdates";
import { SETTINGS_TABS, settingsTabLabel, settingsTabMeta, type SettingsTab } from "./SettingsShared";

// SettingsPanel is the desktop settings surface, aligning with Claude Code's
// settings: model & providers (incl. API keys), permissions, sandbox, agent
// params, and appearance. Every change writes tianxuan.toml (or .env for keys)
// through the kernel's config edit API and rebuilds the controller live.
export function SettingsPanel({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  const t = useT();
  const [s, setS] = useState<SettingsView | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [scheme, setSchemeState] = useState<ColorScheme>(getColorScheme());
  const [mode, setModeState] = useState<ThemeMode>(getThemeMode());
  const [tab, setTab] = useState<SettingsTab>("models");
  const [query, setQuery] = useState("");

  // V8.4.1: 图标映射 + 搜索过滤
  const TAB_ICONS: Record<SettingsTab, React.ReactNode> = {
    models: <Cpu size={14} />,
    providers: <Plug size={14} />,
    permissions: <Shield size={14} />,
    sandbox: <Box size={14} />,
    agent: <Bot size={14} />,
    appearance: <Palette size={14} />,
    updates: <CloudUpload size={14} />,
  };
  const filteredTabs = query.trim() && s
    ? SETTINGS_TABS.filter((id) => {
        const label = settingsTabLabel(id, t).toLowerCase();
        const meta = settingsTabMeta(id, s, t).toLowerCase();
        return label.includes(query.toLowerCase()) || meta.includes(query.toLowerCase());
      })
    : SETTINGS_TABS;

  const reload = async () => setS(await app.Settings().catch(() => null));
  useEffect(() => {
    void reload();
  }, []);

  // apply runs a mutation, re-reads settings, and refreshes the topbar/model. A
  // rejected binding (validation / rebuild failure) surfaces as an inline banner.
  const apply = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      await reload();
      onChanged();
    } catch (e) {
      setErr(String((e as Error)?.message ?? e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <ResizableDrawer onClose={onClose} wide>
        <DrawerHeader onClose={onClose}>
          <DrawerTitle text={t("settings.title")} />
        </DrawerHeader>

        {!s ? (
          <div className="empty-state">{t("settings.loading")}</div>
        ) : (
          <div className="flex-1 min-h-0 flex h-full overflow-y-auto">
            <div className="flex h-full">
              <nav className="flex flex-col gap-1 w-[200px] py-2.5 px-2 border-r border-border-soft overflow-y-auto shrink-0" aria-label={t("settings.title")}>
                {/* 搜索 */}
                <div className="relative mb-1.5">
                  <input
                    className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] pl-7 pr-2 py-1 outline-none placeholder:text-fg-faint/50 focus:border-accent transition-colors"
                    placeholder="搜索…"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                  />
                  <svg className="absolute left-2 top-1/2 -translate-y-1/2 text-fg-faint/40" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
                </div>
                {filteredTabs.length === 0 ? (
                  <div className="px-3 py-4 text-center text-[11px] text-fg-faint">无匹配</div>
                ) : (
                  filteredTabs.map((id) => (
                    <button
                      key={id}
                      className={`flex items-center gap-2 w-full px-3 py-2 border-0 rounded-lg bg-transparent text-left cursor-pointer transition-[color,background] duration-[var(--dur-fast)] ${
                        tab === id ? "text-accent bg-accent-soft" : "text-fg-dim hover:text-fg hover:bg-bg-soft"
                      }`}
                      onClick={() => { setTab(id); setQuery(""); }}
                    >
                      <span className="shrink-0 opacity-70">{TAB_ICONS[id]}</span>
                      <div className="flex flex-col gap-0.5 min-w-0">
                        <span className="text-[13px] font-medium">{settingsTabLabel(id, t)}</span>
                        <small className="text-[11px] text-fg-faint truncate">{settingsTabMeta(id, s, t)}</small>
                      </div>
                    </button>
                  ))
                )}
              </nav>
              <main className="flex-1 min-w-0 overflow-y-auto px-5 py-2.5">
                {err && <div className="shrink-0 px-4 py-2 text-[12.5px] bg-del-bg text-err border-b border-border-soft">{err}</div>}
                {tab === "models" && <ModelsSection s={s} busy={busy} apply={apply} onManageProviders={() => setTab("providers")} />}
                {tab === "providers" && <ProvidersSection s={s} busy={busy} apply={apply} />}
                {tab === "permissions" && <PermissionsSection s={s} busy={busy} apply={apply} />}
                {tab === "sandbox" && <SandboxSection s={s} busy={busy} apply={apply} />}
                {tab === "agent" && <AgentSection s={s} busy={busy} apply={apply} />}
                {tab === "appearance" && (
                  <AppearanceSection
                    scheme={scheme}
                    mode={mode}
                    onScheme={(s) => { applyColorScheme(s); setSchemeState(s); }}
                    onMode={(m) => { applyThemeMode(m); setModeState(m); }}
                  />
                )}
                {tab === "updates" && <UpdatesSection configPath={s.configPath} />}
              </main>
            </div>
          </div>
        )}
    </ResizableDrawer>
  );
}
