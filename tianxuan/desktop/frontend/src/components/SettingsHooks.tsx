import { SettingsPageShell } from "./SettingsPageShell";

export function SettingsHooks() {
  return (
    <SettingsPageShell title="钩子" desc="事件驱动的脚本钩子，在会话生命周期特定时刻自动执行。">
      <div className="text-[12px] text-fg-faint leading-relaxed">
        <p>钩子通过 .tianxuan/settings.json 或 ~/.tianxuan/settings.json 配置，支持以下事件：</p>
        <ul className="list-disc pl-5 mt-2 space-y-1 font-mono text-[11px]">
          <li>SessionStart — 会话开始</li>
          <li>SessionEnd — 会话结束</li>
          <li>PreToolUse — 工具调用前</li>
          <li>PostToolUse — 工具调用后</li>
          <li>Stop — 停止时</li>
          <li>PreCompact — 压缩前</li>
          <li>PostLLMCall — LLM 调用后</li>
          <li>Notification — 通知</li>
          <li>SubagentStop — 子代理停止</li>
        </ul>
        <p className="mt-3 text-[11px] opacity-70">完整钩子管理面板将在后续版本中提供。使用 `reasonix hooks list` 命令查看已加载的钩子。</p>
      </div>
    </SettingsPageShell>
  );
}
