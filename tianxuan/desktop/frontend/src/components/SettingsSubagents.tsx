import { SettingsPageShell } from "./SettingsPageShell";

export function SettingsSubagents() {
  return (
    <SettingsPageShell title="子代理" desc="管理 runAs=subagent 技能的默认模型和工具白名单。">
      <div className="text-[12px] text-fg-faint leading-relaxed">
        <p>子代理模型和工具权限通过以下方式控制：</p>
        <ul className="list-disc pl-5 mt-2 space-y-1">
          <li><strong>默认模型</strong>：在「模型」tab 设置全局子代理模型，或在「智能体」tab 按技能覆盖</li>
          <li><strong>递归深度</strong>：在「通用」tab 限制子代理嵌套层数</li>
          <li><strong>推理力度</strong>：在「模型」tab 设置子代理 Effort 级别</li>
        </ul>
        <p className="mt-3 text-[11px] opacity-70">完整子代理管理面板将在后续版本中提供。</p>
      </div>
    </SettingsPageShell>
  );
}
