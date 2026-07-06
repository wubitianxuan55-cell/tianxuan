import { useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { toRef, type SectionProps } from "./SettingsShared";
import type { ProviderView } from "../lib/types";

export function ProvidersSection({ s, busy, apply }: SectionProps) {
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
