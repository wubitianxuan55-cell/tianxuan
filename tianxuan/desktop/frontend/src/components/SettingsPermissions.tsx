import { useState } from "react";
import { Shield, X, ShieldBan, ShieldQuestion, ShieldCheck, ChevronRight, ChevronDown } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { SectionProps } from "./SettingsShared";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";

export function PermissionsSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><Shield size={15} className="text-accent" />权限</span>} desc="控制工具写入前是否需要确认，以及各模式下的规则列表。">
      <SettingsSection title={<span className="flex items-center gap-1.5"><ShieldQuestion size={13} className="text-accent" />写入模式</span>}>
        <SettingsField label="全局模式" hint="ask=询问 / allow=自动放行 / deny=拒绝。">
          <select
            className="bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent min-w-[160px]"
            value={s.permissions.mode}
            disabled={busy}
            onChange={(e) => void apply(() => app.SetPermissionMode(e.target.value))}
          >
            <option value="ask">{t("settings.modeAsk")}</option>
            <option value="allow">{t("settings.modeAllow")}</option>
            <option value="deny">{t("settings.modeDeny")}</option>
          </select>
        </SettingsField>
      </SettingsSection>

      <SettingsSection title={<span className="flex items-center gap-1.5"><Shield size={13} className="text-accent" />工具规则</span>}>
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
        <div className="text-fg-faint text-[10px] mt-1">{t("settings.ruleForm")}</div>
      </SettingsSection>
    </SettingsPageShell>
  );
}

function AccordionRuleList({
  list, rules, busy, defaultOpen, onAdd, onRemove,
}: {
  list: string; rules: string[]; busy: boolean; defaultOpen: boolean;
  onAdd: (rule: string) => Promise<void>; onRemove: (rule: string) => Promise<void>;
}) {
  const t = useT();
  const [open, setOpen] = useState(defaultOpen);
  const [draft, setDraft] = useState("");
  const add = () => { const r = draft.trim(); if (r) { void onAdd(r); setDraft(""); } };

  const icons: Record<string, React.ReactNode> = {
    deny: <ShieldBan size={13} className="text-red-400" />,
    ask: <ShieldQuestion size={13} className="text-amber-400" />,
    allow: <ShieldCheck size={13} className="text-emerald-400" />,
  };
  const labels: Record<string, string> = {
    deny: "deny（拒绝）", ask: "ask（询问）", allow: "allow（允许）",
  };

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <button
        className="flex items-center gap-2 w-full px-3 py-2 bg-transparent border-0 text-left cursor-pointer hover:bg-bg transition-colors"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="shrink-0 text-fg-faint transition-transform duration-150">{open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}</span>
        {icons[list]}
        <span className="text-fg-dim text-[12px] font-medium">{labels[list]}</span>
        {rules.length > 0 && (
          <span className="ml-auto text-[10px] font-mono text-fg-faint bg-bg px-1.5 py-px rounded">{rules.length}</span>
        )}
      </button>
      {open && (
        <div className="px-3 pb-2 border-t border-border-soft bg-bg/60">
          <div className="flex flex-wrap gap-1.5 py-2">
            {rules.length === 0 && <span className="text-fg-faint text-[11px] italic">{t("common.none")}</span>}
            {rules.map((r) => (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 border border-border-soft rounded text-fg-dim text-[11px] bg-bg" key={r}>
                {r}
                <button className="ml-0.5 w-4 h-4 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:bg-bg-elev transition-colors" disabled={busy} onClick={() => void onRemove(r)} title={t("common.delete")}>
                  <X size={11} />
                </button>
              </span>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <input
              className="flex-1 bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
              placeholder={t("settings.addRule", { list })}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") add(); }}
            />
            <button className="btn btn--small" disabled={busy || !draft.trim()} onClick={add}>{t("common.add")}</button>
          </div>
        </div>
      )}
    </div>
  );
}

export function RuleList({
  list, rules, busy, onAdd, onRemove,
}: {
  list: string; rules: string[]; busy: boolean;
  onAdd: (rule: string) => Promise<void>; onRemove: (rule: string) => Promise<void>;
}) {
  const t = useT();
  const [draft, setDraft] = useState("");
  const add = () => { const r = draft.trim(); if (r) { void onAdd(r); setDraft(""); } };
  return (
    <div className="mb-2">
      <div className="text-fg-dim text-[12px] font-medium mb-1">{list}</div>
      <div className="flex flex-wrap gap-1.5">
        {rules.length === 0 && <span className="text-fg-faint text-xs">{t("common.none")}</span>}
        {rules.map((r) => (
          <span className="inline-flex items-center gap-1 px-2 py-0.5 border border-border-soft rounded text-fg-dim text-[11px] bg-bg" key={r}>
            {r}
            <button className="ml-0.5 w-4 h-4 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:bg-bg-elev transition-colors" disabled={busy} onClick={() => void onRemove(r)} title={t("common.delete")}>
              <X size={11} />
            </button>
          </span>
        ))}
      </div>
      <div className="mt-1 flex items-center gap-2">
        <input className="flex-1 bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
          placeholder={t("settings.addRule", { list })} value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") add(); }} />
        <button className="btn btn--small" disabled={busy || !draft.trim()} onClick={add}>{t("common.add")}</button>
      </div>
    </div>
  );
}
