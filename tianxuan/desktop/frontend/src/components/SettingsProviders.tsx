import { useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { toRef, type SectionProps } from "./SettingsShared";

const PROVIDER_PRESETS = [
  { name: "deepseek", kind: "openai", baseUrl: "https://api.deepseek.com", models: "deepseek-chat", apiEnv: "DEEPSEEK_API_KEY", ctx: 1000000 },
  { name: "openai", kind: "openai", baseUrl: "https://api.openai.com/v1", models: "gpt-4o", apiEnv: "OPENAI_API_KEY", ctx: 128000 },
  { name: "anthropic", kind: "openai", baseUrl: "https://api.anthropic.com/v1", models: "claude-sonnet-4-20250514", apiEnv: "ANTHROPIC_API_KEY", ctx: 200000 },
  { name: "kimi", kind: "openai", baseUrl: "https://api.moonshot.cn/v1", models: "moonshot-v1-8k", apiEnv: "MOONSHOT_API_KEY", ctx: 8000 },
  { name: "qwen", kind: "openai", baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1", models: "qwen-plus", apiEnv: "DASHSCOPE_API_KEY", ctx: 131072 },
  { name: "glm", kind: "openai", baseUrl: "https://open.bigmodel.cn/api/paas/v4", models: "glm-4-plus", apiEnv: "ZHIPUAI_API_KEY", ctx: 128000 },
  { name: "ollama", kind: "openai", baseUrl: "http://localhost:11434/v1", models: "llama3.1", apiEnv: "", ctx: 128000 },
];

import type { ProviderView } from "../lib/types";

export function ProvidersSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  // The provider backing the default model — can't be deleted (would dangle the
  // default). default_model may be a provider name or a "provider/model" ref.
  const defaultProvider = toRef(s.defaultModel, s).split("/")[0];
  const [editing, setEditing] = useState<string | null>(null);
  const [quickPreset, setQuickPreset] = useState<typeof PROVIDER_PRESETS[0] | null>(null); // provider name, or "__new__"

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
              onCancel={() => { setEditing(null); setQuickPreset(null); }}
              onSave={(pv) => apply(() => app.SaveProvider(pv)).then(() => setEditing(null))}
            />
          ) : (
            <div className="border border-border-soft rounded-lg p-3 mb-2" key={p.name}>
              <div className="flex items-center gap-2">
                <span className="text-fg text-[13px] font-semibold">{p.name}</span>
                <span className={`badge ${p.keySet ? "badge--success" : "badge--warning"}`}>
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

      {editing === "__new__" && !quickPreset && (
        <div className="flex flex-col gap-2 p-3 border border-border-soft rounded-lg mb-2">
          <div className="text-fg text-[13px] font-semibold mb-1">选择预设模板</div>
          <div className="grid grid-cols-3 gap-2">
            {PROVIDER_PRESETS.map(p => (
              <button key={p.name} className="text-left bg-bg-soft border border-border-soft rounded-md p-2.5 cursor-pointer hover:border-accent hover:bg-accent-soft/10 transition-colors"
                onClick={() => setQuickPreset(p)}>
                <div className="text-[12px] font-medium text-fg">{p.name}</div>
                <div className="text-[10px] text-fg-faint font-mono mt-1">{p.models}</div>
              </button>
            ))}
            <button className="text-left bg-bg-soft border border-border-soft rounded-md p-2.5 cursor-pointer hover:border-accent hover:bg-accent-soft/10 transition-colors"
              onClick={() => setQuickPreset({name:"",kind:"openai",baseUrl:"",models:"",apiEnv:"",ctx:0})}>
              <div className="text-[12px] font-medium text-fg">自定义</div>
              <div className="text-[10px] text-fg-faint mt-1">手动填写</div>
            </button>
          </div>
        </div>
      )}
      {editing === "__new__" && quickPreset && (
        <ProviderEditor
          kinds={s.providerKinds}
          busy={busy}
          preset={quickPreset.name ? quickPreset : undefined}
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
  preset,
}: {
  initial?: ProviderView;
  kinds: string[];
  busy: boolean;
  onCancel: () => void;
  onSave: (p: ProviderView) => void;
  preset?: typeof PROVIDER_PRESETS[0];
}) {
  const t = useT();
  const [name, setName] = useState(initial?.name ?? preset?.name ?? "");
  const [kind, setKind] = useState(initial?.kind ?? preset?.kind ?? kinds[0] ?? "openai");
  const [baseUrl, setBaseUrl] = useState(initial?.baseUrl ?? preset?.baseUrl ?? "");
  const [models, setModels] = useState((initial?.models ?? (preset ? [preset.models] : [])).join(", "));
  const [apiKeyEnv, setApiKeyEnv] = useState(initial?.apiKeyEnv ?? preset?.apiEnv ?? "");
  const [balanceUrl, setBalanceUrl] = useState(initial?.balanceUrl ?? "");
  // Empty when unset so the placeholder (and its "0 = default" hint) reads instead
  // of a bare "0"; saved back as 0.
  const [ctx, setCtx] = useState(initial?.contextWindow ? String(initial.contextWindow) : preset?.ctx ? String(preset.ctx) : "");
  const [thinking, setThinking] = useState(initial?.thinking ?? "");
  const [effort, setEffort] = useState(initial?.effort ?? "");
  const [chatUrl, setChatUrl] = useState(initial?.chatUrl ?? "");
  const [reasoningProtocol, setReasoningProtocol] = useState(initial?.reasoningProtocol ?? "");
  const [defaultEffort, setDefaultEffort] = useState(initial?.defaultEffort ?? "");

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
      thinking: thinking.trim(),
      effort: effort.trim(),
      chatUrl: chatUrl.trim(),
      modelsUrl: "",
      headers: {},
      extraBody: "",
      authHeader: false,
      visionModels: [],
      reasoningProtocol: reasoningProtocol.trim(),
      supportedEfforts: [],
      defaultEffort: defaultEffort.trim(),
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
          <label className="text-fg-dim text-[13px] shrink-0">Thinking 模式</label>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder="enabled / disabled / adaptive（留空=默认）" value={thinking} onChange={(e) => setThinking(e.target.value)} />
          <label className="text-fg-dim text-[13px] shrink-0">Effort 力度</label>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder="low / medium / high / max（留空=默认）" value={effort} onChange={(e) => setEffort(e.target.value)} />
          <label className="text-fg-dim text-[13px] shrink-0">推理协议</label>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder="auto / deepseek / openai / none" value={reasoningProtocol} onChange={(e) => setReasoningProtocol(e.target.value)} />
          <label className="text-fg-dim text-[13px] shrink-0">Chat URL</label>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder="独立 Chat 端点（留空=BaseURL）" value={chatUrl} onChange={(e) => setChatUrl(e.target.value)} />
          <label className="text-fg-dim text-[13px] shrink-0">默认 Effort</label>
          <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder="low / medium / high / max（留空=全局默认）" value={defaultEffort} onChange={(e) => setDefaultEffort(e.target.value)} />
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
