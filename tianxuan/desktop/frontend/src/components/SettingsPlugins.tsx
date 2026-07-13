import { SettingsPageShell } from "./SettingsPageShell";

export function SettingsPlugins() {
  return (
    <SettingsPageShell title="插件" desc="[[plugins]] 配置段声明的外部 MCP 服务器和扩展。">
      <div className="text-[12px] text-fg-faint leading-relaxed">
        <p>插件在 config.toml 的 [[plugins]] 段声明，启动时自动连接。</p>
        <p className="mt-2">已连接的 MCP 服务器可在「MCP」tab 查看状态和工具列表。</p>
        <p className="mt-3 text-[11px] opacity-70">完整插件管理面板将在后续版本中提供。</p>
      </div>
    </SettingsPageShell>
  );
}
