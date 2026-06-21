import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useI18n, useT } from "../lib/i18n";
import { useUpdater } from "../lib/useUpdater";
import { applyTheme, getTheme, type Theme } from "../lib/theme";
import type { ProviderView, SettingsView } from "../lib/types";
import { ResizableDrawer } from "./ResizableDrawer";
import { X, Cpu, Shield, Box, Bot, Palette, CloudUpload, Plug } from "lucide-react";

type SettingsTab = "models" | "providers" | "permissions" | "sandbox" | "agent" | "appearance" | "updates";

const SETTINGS_TABS: SettingsTab[] = ["models", "providers", "permissions", "sandbox", "agent", "appearance", "updates"];

// SettingsPanel is the desktop settings surface, aligning with Claude Code's
// settings: model & providers (incl. API keys), permissions, sandbox, agent
// params, and appearance. Every change writes tianxuan.toml (or .env for keys)
// through the kernel's config edit API and rebuilds the controller live.
export function SettingsPanel({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  const t = useT();
  const [s, setS] = useState<SettingsView | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [theme, setThemeState] = useState<Theme>(getTheme());
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
        <header className="flex items-center justify-between px-4 py-3.5 bg-bg-elev border-b border-border">
          <div className="text-[15px] font-semibold text-fg">{t("settings.title")}</div>
          <button className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border bg-bg-soft text-fg-faint rounded-[7px] cursor-pointer transition-[color,border-color,background] duration-[0.12s] hover:text-fg hover:border-fg-faint no-drag" onClick={onClose} title={t("common.close")}>
            <X size={14} />
          </button>
        </header>

        {!s ? (
          <div className="empty">{t("settings.loading")}</div>
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
                      className={`flex items-center gap-2 w-full px-3 py-2 border-0 rounded-lg bg-transparent text-left cursor-pointer transition-[color,background] duration-[0.12s] ${
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
                    theme={theme}
                    onTheme={(t) => {
                      applyTheme(t);
                      setThemeState(t);
                    }}
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

type SectionProps = {
  s: SettingsView;
  busy: boolean;
  apply: (fn: () => Promise<void>) => Promise<void>;
};

function settingsTabLabel(id: SettingsTab, t: ReturnType<typeof useT>): string {
  switch (id) {
    case "models":
      return t("settings.tab.models");
    case "providers":
      return t("settings.tab.providers");
    case "permissions":
      return t("settings.tab.permissions");
    case "sandbox":
      return t("settings.tab.sandbox");
    case "agent":
      return t("settings.tab.agent");
    case "appearance":
      return t("settings.tab.appearance");
    case "updates":
      return t("settings.tab.updates");
  }
}

function settingsTabMeta(id: SettingsTab, s: SettingsView, t: ReturnType<typeof useT>): string {
  switch (id) {
    case "models":
      return toRef(s.defaultModel, s) || t("common.none");
    case "providers":
      return t("settings.providerCount", { n: s.providers.length });
    case "permissions":
      return s.permissions.mode;
    case "sandbox":
      return s.sandbox.bash;
    case "agent":
      return t("settings.agentMeta", { temp: s.agent.temperature, steps: s.agent.maxSteps || "∞" });
    case "appearance":
      return t("settings.appearanceMeta");
    case "updates":
      return t("settings.updatesMeta");
  }
}

// allRefs flattens providers into "provider/model" refs for the model selectors.
function allRefs(s: SettingsView): string[] {
  const out: string[] = [];
  for (const p of s.providers) for (const m of p.models) out.push(`${p.name}/${m}`);
  return out;
}

// toRef normalises a stored model id (a provider name, a bare model, or a ref) to
// a "provider/model" ref so a <select> of refs can show it selected.
function toRef(model: string, s: SettingsView): string {
  if (!model) return "";
  if (model.includes("/")) return model;
  const byName = s.providers.find((p) => p.name === model);
  if (byName) return `${byName.name}/${byName.default || byName.models[0] || ""}`;
  const byModel = s.providers.find((p) => p.models.includes(model));
  if (byModel) return `${byModel.name}/${model}`;
  return model;
}

function ModelsSection({ s, busy, apply, onManageProviders }: SectionProps & { onManageProviders: () => void }) {
  const t = useT();
  const refs = allRefs(s);
  const defaultRef = toRef(s.defaultModel, s);
  const [defaultProvider, defaultModel] = defaultRef.split("/");

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold px-1 pb-1.5">{t("settings.tab.models")}</div>

      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("settings.defaultModel")}</label>
        <select
          className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent flex-1 min-w-0"
          value={toRef(s.defaultModel, s)}
          disabled={busy}
          onChange={(e) => void apply(() => app.SetDefaultModel(e.target.value))}
        >
          {refs.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
      </div>

      <div className="flex items-center gap-2 px-3 py-2 border border-border-soft rounded-lg mb-3">
        <span className="text-fg-faint text-[11px] shrink-0">{t("settings.activeProvider")}</span>
        <span className="text-[11px] font-semibold text-fg">{defaultProvider || t("common.none")}</span>
        <span className="text-border mx-0.5">·</span>
        <span className="text-[11px] font-mono text-fg-dim">{defaultModel || defaultRef || t("common.none")}</span>
        <span className="flex-1" />
        <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={onManageProviders}>
          {t("settings.manageProviders")}
        </button>
      </div>
    </section>
  );
}

function ProvidersSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  // The provider backing the default model — can't be deleted (would dangle the
  // default). default_model may be a provider name or a "provider/model" ref.
  const defaultProvider = toRef(s.defaultModel, s).split("/")[0];
  const [editing, setEditing] = useState<string | null>(null); // provider name, or "__new__"

  return (
    <section className="mb-3">
      <div className="flex items-center justify-between px-1 pb-1.5">
        <div className="text-fg text-sm font-semibold">{t("settings.tab.providers")}</div>
        {editing !== "__new__" && (
          <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" disabled={busy} onClick={() => setEditing("__new__")}>
            {t("settings.addProvider")}
          </button>
        )}
      </div>

      <div className="flex flex-col gap-2">
        {s.providers.map((p) =>
          editing === p.name ? (
            <ProviderEditor
              key={p.name}
              initial={p}
              kinds={s.providerKinds}
              busy={busy}
              onCancel={() => setEditing(null)}
              onSave={(pv) => apply(() => app.SaveProvider(pv)).then(() => setEditing(null))}
            />
          ) : (
            <div className="border border-border-soft rounded-lg p-3 mb-2" key={p.name}>
              <div className="flex items-center gap-2">
                <span className="text-fg text-[13px] font-semibold">{p.name}</span>
                <span className={`badge ${p.keySet ? "badge--project" : "badge--feedback"}`}>
                  {p.keySet ? t("settings.keySet") : t("settings.noKey")}
                </span>
                <span className="flex-1" />
                <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={() => setEditing(p.name)}>
                  {t("common.edit")}
                </button>
                <button
                  className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
                  disabled={busy || defaultProvider === p.name}
                  title={defaultProvider === p.name ? t("settings.cantDeleteDefault") : t("settings.deleteProvider")}
                  onClick={() => void apply(() => app.DeleteProvider(p.name))}
                >
                  {t("common.delete")}
                </button>
              </div>
              <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-fg-faint text-[11px] mt-1">
                <span className="font-mono text-[10px]">{p.kind}</span>
                <span className="truncate max-w-[260px]" title={p.baseUrl}>{p.baseUrl}</span>
                <span className="truncate max-w-[260px]" title={p.models.join(", ")}>{p.models.join(", ")}</span>
              </div>
              <KeyField apiKeyEnv={p.apiKeyEnv} busy={busy} onSet={(v) => apply(() => app.SetProviderKey(p.apiKeyEnv, v))} />
            </div>
          ),
        )}
      </div>

      {editing === "__new__" && (
        <ProviderEditor
          kinds={s.providerKinds}
          busy={busy}
          onCancel={() => setEditing(null)}
          onSave={(pv) => apply(() => app.SaveProvider(pv)).then(() => setEditing(null))}
        />
      )}
    </section>
  );
}

function ProviderEditor({
  initial,
  kinds,
  busy,
  onCancel,
  onSave,
}: {
  initial?: ProviderView;
  kinds: string[];
  busy: boolean;
  onCancel: () => void;
  onSave: (p: ProviderView) => void;
}) {
  const t = useT();
  const [name, setName] = useState(initial?.name ?? "");
  const [kind, setKind] = useState(initial?.kind ?? kinds[0] ?? "openai");
  const [baseUrl, setBaseUrl] = useState(initial?.baseUrl ?? "");
  const [models, setModels] = useState((initial?.models ?? []).join(", "));
  const [apiKeyEnv, setApiKeyEnv] = useState(initial?.apiKeyEnv ?? "");
  const [balanceUrl, setBalanceUrl] = useState(initial?.balanceUrl ?? "");
  // Empty when unset so the placeholder (and its "0 = default" hint) reads instead
  // of a bare "0"; saved back as 0.
  const [ctx, setCtx] = useState(initial?.contextWindow ? String(initial.contextWindow) : "");

  // Offer the kinds the kernel actually registered; if the stored kind is a
  // legacy/unknown one, keep it as an option so editing doesn't silently change it.
  const kindOptions = kind && !kinds.includes(kind) ? [kind, ...kinds] : kinds;

  const save = () => {
    const ms = models
      .split(",")
      .map((m) => m.trim())
      .filter(Boolean);
    onSave({
      name: name.trim(),
      kind: kind.trim() || kinds[0] || "openai",
      baseUrl: baseUrl.trim(),
      models: ms,
      default: ms[0] ?? "",
      apiKeyEnv: apiKeyEnv.trim(),
      keySet: initial?.keySet ?? false,
      balanceUrl: balanceUrl.trim(),
      contextWindow: Number(ctx) || 0,
    });
  };

  return (
    <div className="flex flex-col gap-2 p-3 border border-border-soft rounded-lg mb-2">
      {/* 基本设置 */}
      <fieldset className="border border-border-soft rounded-md p-2.5">
        <legend className="text-fg-faint text-[10px] font-medium px-1">基本设置</legend>
        <div className="flex flex-col gap-2">
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("settings.providerName")} value={name} onChange={(e) => setName(e.target.value)} disabled={!!initial} />
          <div className="flex items-center gap-3">
            <label className="text-fg-dim text-[13px] shrink-0">{t("settings.providerKind")}</label>
            <select className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={kind} onChange={(e) => setKind(e.target.value)}>
              {kindOptions.map((k) => (
                <option key={k} value={k}>{k}</option>
              ))}
            </select>
          </div>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("settings.providerBaseUrl")} value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} />
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("settings.providerModels")} value={models} onChange={(e) => setModels(e.target.value)} />
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("settings.providerApiKeyEnv")} value={apiKeyEnv} onChange={(e) => setApiKeyEnv(e.target.value)} />
        </div>
      </fieldset>
      {/* 高级设置 */}
      <fieldset className="border border-border-soft rounded-md p-2.5">
        <legend className="text-fg-faint text-[10px] font-medium px-1">高级设置</legend>
        <div className="flex flex-col gap-2">
          <label className="text-fg-dim text-[13px] shrink-0">{t("settings.providerBalanceUrl")}</label>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("settings.balanceUrlPlaceholder")} value={balanceUrl} onChange={(e) => setBalanceUrl(e.target.value)} />
          <div className="text-fg-faint text-[10px]">{t("settings.balanceUrlHint")}</div>
          <label className="text-fg-dim text-[13px] shrink-0">{t("settings.providerContextWindow")}</label>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("settings.contextWindowPlaceholder")} value={ctx} onChange={(e) => setCtx(e.target.value)} inputMode="numeric" />
          <div className="text-fg-faint text-[10px]">{t("settings.contextWindowHint")}</div>
        </div>
      </fieldset>
      <div className="flex gap-2 mt-2">
        <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={onCancel} disabled={busy}>
          {t("common.cancel")}
        </button>
        <button className="btn--primary" onClick={save} disabled={busy || !name.trim() || !baseUrl.trim()}>
          {t("common.save")}
        </button>
      </div>
    </div>
  );
}

function KeyField({ apiKeyEnv, busy, onSet }: { apiKeyEnv: string; busy: boolean; onSet: (v: string) => Promise<void> }) {
  const t = useT();
  const [val, setVal] = useState("");
  if (!apiKeyEnv) return null;
  return (
    <div className="flex items-center gap-2 mt-2">
      <input
        className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
        type="password"
        placeholder={t("settings.setKey", { env: apiKeyEnv })}
        value={val}
        onChange={(e) => setVal(e.target.value)}
      />
      <button
        className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
        disabled={busy || !val.trim()}
        onClick={() => {
          void onSet(val.trim());
          setVal("");
        }}
      >
        {t("settings.saveKey")}
      </button>
    </div>
  );
}

function PermissionsSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold">{t("settings.permissions")}</div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("settings.writerMode")}</label>
        <select
          className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent flex-1 min-w-0"
          value={s.permissions.mode}
          disabled={busy}
          onChange={(e) => void apply(() => app.SetPermissionMode(e.target.value))}
        >
          <option value="ask">{t("settings.modeAsk")}</option>
          <option value="allow">{t("settings.modeAllow")}</option>
          <option value="deny">{t("settings.modeDeny")}</option>
        </select>
      </div>
      <div className="flex flex-col gap-2">
        {(["deny", "ask", "allow"] as const).map((list) => (
          <AccordionRuleList
            key={list}
            list={list}
            rules={s.permissions[list]}
            busy={busy}
            defaultOpen={list === "deny" || (list === "ask" && s.permissions.deny.length === 0)}
            onAdd={(rule) => apply(() => app.AddPermissionRule(list, rule))}
            onRemove={(rule) => apply(() => app.RemovePermissionRule(list, rule))}
          />
        ))}
      </div>
      <div className="text-fg-faint text-[10px] mt-1 px-1">{t("settings.ruleForm")}</div>
    </section>
  );
}

function AccordionRuleList({
  list,
  rules,
  busy,
  defaultOpen,
  onAdd,
  onRemove,
}: {
  list: string;
  rules: string[];
  busy: boolean;
  defaultOpen: boolean;
  onAdd: (rule: string) => Promise<void>;
  onRemove: (rule: string) => Promise<void>;
}) {
  const t = useT();
  const [open, setOpen] = useState(defaultOpen);
  const [draft, setDraft] = useState("");
  const add = () => {
    const r = draft.trim();
    if (r) { void onAdd(r); setDraft(""); }
  };
  const listLabel = list === "deny" ? "🚫 deny（拒绝）" : list === "ask" ? "❓ ask（询问）" : "✅ allow（允许）";
  const count = rules.length;

  return (
    <div className="border border-border-soft rounded-lg overflow-hidden mb-1.5">
      <button
        className="flex items-center gap-2 w-full px-3 py-2 bg-transparent border-0 text-left cursor-pointer hover:bg-bg-soft transition-colors"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="text-[10px] text-fg-faint transition-transform duration-150" style={{ transform: open ? "rotate(90deg)" : "rotate(0deg)" }}>▶</span>
        <span className="text-fg-dim text-[12px] font-medium">{listLabel}</span>
        {count > 0 && (
          <span className="ml-auto text-[10px] font-mono text-fg-faint bg-bg-elev px-1.5 py-px rounded">{count}</span>
        )}
      </button>
      {open && (
        <div className="px-3 pb-2 border-t border-border-soft">
          <div className="flex flex-wrap gap-1.5 py-2">
            {rules.length === 0 && <span className="text-fg-faint text-[11px] italic">{t("common.none")}</span>}
            {rules.map((r) => (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 border border-border-soft rounded text-fg-dim text-[11px] bg-bg-soft" key={r}>
                {r}
                <button className="ml-0.5 w-4 h-4 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:bg-bg-elev transition-colors" disabled={busy} onClick={() => void onRemove(r)} title={t("common.delete")}>
                  <X size={11} />
                </button>
              </span>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <input
              className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
              placeholder={t("settings.addRule", { list })}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") add(); }}
            />
            <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors shrink-0" disabled={busy || !draft.trim()} onClick={add}>
              {t("common.add")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function RuleList({
  list,
  rules,
  busy,
  onAdd,
  onRemove,
}: {
  list: string;
  rules: string[];
  busy: boolean;
  onAdd: (rule: string) => Promise<void>;
  onRemove: (rule: string) => Promise<void>;
}) {
  const t = useT();
  const [draft, setDraft] = useState("");
  const add = () => {
    const r = draft.trim();
    if (r) {
      void onAdd(r);
      setDraft("");
    }
  };
  return (
    <div className="mb-2">
      <div className="text-fg-dim text-[12px] font-medium mb-1">{list}</div>
      <div className="flex flex-wrap gap-1.5">
        {rules.length === 0 && <span className="text-fg-faint text-xs">{t("common.none")}</span>}
        {rules.map((r) => (
          <span className="inline-flex items-center gap-1 px-2 py-0.5 border border-border-soft rounded text-fg-dim text-[11px] bg-bg-soft" key={r}>
            {r}
            <button className="ml-0.5 w-4 h-4 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:bg-bg-elev transition-colors" disabled={busy} onClick={() => void onRemove(r)} title={t("common.delete")}>
              <X size={11} />
            </button>
          </span>
        ))}
      </div>
      <div className="mt-1">
        <input
          className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
          placeholder={t("settings.addRule", { list })}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") add();
          }}
        />
        <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" disabled={busy || !draft.trim()} onClick={add}>
          {t("common.add")}
        </button>
      </div>
    </div>
  );
}

function SandboxSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  const sb = s.sandbox;
  const [root, setRoot] = useState(sb.workspaceRoot);
  const set = (next: Partial<typeof sb>) =>
    apply(() => app.SetSandbox(next.bash ?? sb.bash, next.network ?? sb.network, next.workspaceRoot ?? sb.workspaceRoot, next.allowWrite ?? sb.allowWrite));

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold">{t("settings.sandboxTitle")}</div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("settings.bashSandbox")}</label>
        <select className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent flex-1 min-w-0" value={sb.bash} disabled={busy} onChange={(e) => void set({ bash: e.target.value })}>
          <option value="enforce">{t("settings.bashEnforce")}</option>
          <option value="off">{t("settings.bashOff")}</option>
        </select>
      </div>
      <label className="flex items-center gap-2 text-fg-dim text-[13px] cursor-pointer">
        <input type="checkbox" checked={sb.network} disabled={busy} onChange={(e) => void set({ network: e.target.checked })} />
        {t("settings.allowNetwork")}
      </label>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("settings.workspaceRoot")}</label>
        <input
          className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
          placeholder={t("settings.workspaceDefault")}
          value={root}
          disabled={busy}
          onChange={(e) => setRoot(e.target.value)}
          onBlur={() => root !== sb.workspaceRoot && void set({ workspaceRoot: root })}
        />
      </div>
      <RuleList
        list="allow_write"
        rules={sb.allowWrite}
        busy={busy}
        onAdd={(d) => set({ allowWrite: [...sb.allowWrite, d] })}
        onRemove={(d) => set({ allowWrite: sb.allowWrite.filter((x) => x !== d) })}
      />
    </section>
  );
}

function AgentSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  const [temp, setTemp] = useState(String(s.agent.temperature));
  const [steps, setSteps] = useState(String(s.agent.maxSteps));
  const [prompt, setPrompt] = useState(s.agent.systemPrompt);
  const dirty = temp !== String(s.agent.temperature) || steps !== String(s.agent.maxSteps) || prompt !== s.agent.systemPrompt;

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold">{t("settings.agent")}</div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] w-[80px] shrink-0">{t("settings.temperature")}</label>
        <input className="w-[70px] bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent text-center" value={temp} onChange={(e) => setTemp(e.target.value)} disabled={busy} inputMode="decimal" />
        <span className="text-fg-faint text-[10px]">0.0–1.0</span>
      </div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] w-[80px] shrink-0">{t("settings.maxSteps")}</label>
        <input className="w-[70px] bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent text-center" value={steps} onChange={(e) => setSteps(e.target.value)} disabled={busy} inputMode="numeric" />
        <span className="text-fg-faint text-[10px]">{t("settings.unlimited")}</span>
      </div>
      <div className="text-fg-dim text-[12px] font-medium mb-1">{t("settings.systemPrompt")}</div>
      <textarea className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] p-2.5 outline-none resize-y min-h-[120px] focus:border-accent" value={prompt} onChange={(e) => setPrompt(e.target.value)} disabled={busy} spellCheck={false} />
      <div className="flex gap-2 mt-2">
        <button
          className="btn--primary"
          disabled={busy || !dirty}
          onClick={() => void apply(() => app.SetAgentParams(Number(temp) || 0, Number(steps) || 0, prompt))}
        >
          {t("settings.saveAgent")}
        </button>
      </div>
    </section>
  );
}

function AppearanceSection({ theme, onTheme }: { theme: Theme; onTheme: (t: Theme) => void }) {
  const { t, pref, setPref } = useI18n();
  const themeOptions: Theme[] = ["auto", "light", "dark", "warm", "ice"];
  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold">{t("settings.appearance")}</div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("settings.theme")}</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {themeOptions.map((opt) => (
            <button
              key={opt}
              className={`px-3 py-1.5 bg-transparent border-0 border-r border-border-soft text-fg-dim text-xs cursor-pointer transition-[color,background] duration-[0.12s] hover:text-fg hover:bg-bg-soft last:border-r-0 ${theme === opt ? "bg-accent-soft text-accent" : ""}`}
              onClick={() => onTheme(opt)}
            >
              {themeName(opt, t)}
            </button>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("settings.language")}</label>
        <select className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent flex-1 min-w-0" value={pref} onChange={(e) => setPref(e.target.value as "" | "en" | "zh" | "zh-TW")}>
          <option value="">{t("settings.langAuto")}</option>
          <option value="zh">简体中文</option>
          <option value="zh-TW">繁體中文</option>
          <option value="en">English</option>
        </select>
      </div>
    </section>
  );
}

function themeName(theme: Theme, t: ReturnType<typeof useT>): string {
  switch (theme) {
    case "auto":
      return t("settings.themeAuto");
    case "light":
      return t("settings.themeLight");
    case "dark":
      return t("settings.themeDark");
    case "warm":
      return "暖护眼";
    case "ice":
      return "冰蓝";
    default:
      return theme;
  }
}

const MB = 1024 * 1024;
const mb = (n: number) => (n / MB).toFixed(1);

// UpdatesSection is the manual side of the auto-updater: it shows the running
// version and a Check button, then the same state machine the top banner uses
// (useUpdater) — available → install/download, with progress and errors inline.
function UpdatesSection({ configPath }: { configPath: string }) {
  const t = useT();
  const { status, check, apply } = useUpdater();
  const [version, setVersion] = useState("");
  useEffect(() => {
    app.Version().then(setVersion).catch(() => {});
  }, []);

  const busy =
    status.kind === "checking" || status.kind === "downloading" || status.kind === "verifying" || status.kind === "applying";

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold">{t("updater.title")}</div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("updater.currentVersion", { v: version || "…" })}</label>
        <span className="flex-1" />
        <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" disabled={busy} onClick={() => void check()}>
          {status.kind === "checking" ? t("updater.checking") : t("updater.checkButton")}
        </button>
      </div>
      {status.kind === "upToDate" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.upToDate")}</div>}
      {status.kind === "available" && (
        <>
          <div className="flex items-center gap-3 mb-2.5">
            <span className="text-fg-dim text-[13px] shrink-0">{t("updater.available", { v: status.info.latest })}</span>
            <span className="flex-1" />
            <button className="btn--primary" onClick={() => apply(status.info)}>
              {status.info.canSelfUpdate ? t("updater.installNow") : t("updater.goToDownload")}
            </button>
          </div>
          {!status.info.canSelfUpdate && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.macHint")}</div>}
        </>
      )}
      {status.kind === "downloading" && (
        <div className="text-fg-faint text-[10px] mt-1 px-1">
          {t("updater.downloading", {
            done: mb(status.received),
            total: mb(status.total),
            pct: status.total > 0 ? Math.round((status.received / status.total) * 100) : 0,
          })}
        </div>
      )}
      {status.kind === "verifying" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.verifying")}</div>}
      {status.kind === "applying" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.applying")}</div>}
      {status.kind === "done" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.done")}</div>}
      {status.kind === "error" && <div className="shrink-0 px-4 py-2 text-[12.5px] bg-del-bg text-err border-b border-border-soft">{t("updater.failed", { msg: status.message })}</div>}
      {configPath && (
        <div className="text-fg-faint text-[10px] mt-1 px-1 font-mono truncate" title={configPath}>
          {t("settings.config", { path: configPath })}
        </div>
      )}
    </section>
  );
}
