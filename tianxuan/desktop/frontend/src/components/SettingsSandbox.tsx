import { useState } from "react";
import { Box } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { RuleList } from "./SettingsPermissions";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";
import type { SectionProps } from "./SettingsShared";

export function SandboxSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  const sb = s.sandbox;
  const [root, setRoot] = useState(sb.workspaceRoot);
  const set = (next: Partial<typeof sb>) =>
    apply(() => app.SetSandbox(next.bash ?? sb.bash, next.network ?? sb.network, next.workspaceRoot ?? sb.workspaceRoot, next.allowWrite ?? sb.allowWrite));

  const shellOptions = [
    { value: "auto", label: "自动检测" },
    { value: "bash", label: "Bash" },
    { value: "powershell", label: "PowerShell" },
    { value: "pwsh", label: "PWSh (Core)" },
  ];

  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><Box size={15} />沙箱</span>} desc="控制子进程执行环境、网络访问和工作区根目录。">
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

      <SettingsSection title="运行环境">
        <SettingsField label="沙箱模式" hint="enforce=强制沙箱隔离 / off=关闭沙箱。">
          <select className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent min-w-[160px]"
            value={sb.bash} disabled={busy}
            onChange={(e) => void set({ bash: e.target.value })}>
            <option value="enforce">{t("settings.bashEnforce")}</option>
            <option value="off">{t("settings.bashOff")}</option>
          </select>
        </SettingsField>

        <SettingsField label="网络访问" hint="允许子进程访问外部网络。">
          <label className="flex items-center gap-2 text-[13px] cursor-pointer">
            <input type="checkbox" checked={sb.network} disabled={busy}
              onChange={(e) => void set({ network: e.target.checked })} />
            <span className="text-fg-dim">{t("settings.allowNetwork")}</span>
          </label>
        </SettingsField>

        <SettingsField label="工作区根目录" hint="子进程可见的文件系统根目录。">
          <input
            className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent w-full"
            placeholder={t("settings.workspaceDefault")}
            value={root}
            disabled={busy}
            onChange={(e) => setRoot(e.target.value)}
            onBlur={() => root !== sb.workspaceRoot && void set({ workspaceRoot: root })}
          />
        </SettingsField>
      </SettingsSection>

      <SettingsSection title="写入白名单">
        <RuleList
          list="allow_write"
          rules={sb.allowWrite}
          busy={busy}
          onAdd={(d) => set({ allowWrite: [...sb.allowWrite, d] })}
          onRemove={(d) => set({ allowWrite: sb.allowWrite.filter((x) => x !== d) })}
        />
      </SettingsSection>
    </SettingsPageShell>
  );
}
