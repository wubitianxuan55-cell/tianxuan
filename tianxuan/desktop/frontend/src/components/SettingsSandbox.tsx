import { useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { RuleList } from "./SettingsPermissions";
import { SettingsSection, SettingsField } from "./SettingsPageShell";
import type { SectionProps } from "./SettingsShared";

export function SandboxSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  const sb = s.sandbox;
  const [root, setRoot] = useState(sb.workspaceRoot);
  const set = (next: Partial<typeof sb>) =>
    apply(() => app.SetSandbox(next.bash ?? sb.bash, next.network ?? sb.network, next.workspaceRoot ?? sb.workspaceRoot, next.allowWrite ?? sb.allowWrite));

  // Determine current shell from config; fall back to "auto"
  const shellOptions = [
    { value: "auto", label: "自动检测" },
    { value: "bash", label: "Bash" },
    { value: "powershell", label: "PowerShell" },
    { value: "pwsh", label: "PWSh (Core)" },
  ];

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold mb-3">{t("settings.sandboxTitle")}</div>

      {/* Shell type */}
      <div className="mb-3">
        <SettingsSection title="Shell">
          <SettingsField label="Shell 类型" hint="选择子进程使用的 shell 解释器。">
            <select
              className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent min-w-[140px]"
              value={sb.bash || "auto"}
              disabled={busy}
              onChange={(e) => {
                const shell = e.target.value;
                void apply(() => app.SetShellPreference(shell));
                void set({ bash: shell });
              }}
            >
              {shellOptions.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </SettingsField>
        </SettingsSection>
      </div>

      {/* Bash sandbox mode */}
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">沙箱模式</label>
        <select className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent flex-1 min-w-0" value={sb.bash} disabled={busy} onChange={(e) => void set({ bash: e.target.value })}>
          <option value="enforce">{t("settings.bashEnforce")}</option>
          <option value="off">{t("settings.bashOff")}</option>
        </select>
      </div>

      {/* Network access */}
      <label className="flex items-center gap-2 text-fg-dim text-[13px] cursor-pointer mb-2.5">
        <input type="checkbox" checked={sb.network} disabled={busy} onChange={(e) => void set({ network: e.target.checked })} />
        {t("settings.allowNetwork")}
      </label>

      {/* Workspace root */}
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

      {/* Allow-write rules */}
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
