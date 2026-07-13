import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";

interface ShortcutItem { keys: string; action: string; context: string }

const SHORTCUTS: ShortcutItem[] = [
  { keys: "⌘/Ctrl + N", action: "新建会话", context: "非运行中" },
  { keys: "⌘/Ctrl + K", action: "打开命令面板", context: "全局" },
  { keys: "⌘/Ctrl + ,", action: "打开设置", context: "全局" },
  { keys: "⌘/Ctrl + Shift + M", action: "打开记忆面板", context: "全局" },
  { keys: "⌘/Ctrl + Shift + H", action: "打开历史记录", context: "全局" },
  { keys: "⌘/Ctrl + B", action: "切换侧边栏", context: "全局" },
  { keys: "⌘/Ctrl + J", action: "切换工作区面板", context: "全局" },
  { keys: "Esc", action: "关闭面板 / 取消操作", context: "有面板打开时" },
  { keys: "Enter", action: "发送消息", context: "输入框聚焦" },
  { keys: "Shift + Enter", action: "换行", context: "输入框聚焦" },
  { keys: "Tab", action: "补全路径 / 命令", context: "输入框聚焦" },
  { keys: "↑ / ↓", action: "浏览命令补全 / 历史项", context: "菜单打开时" },
];

export function SettingsShortcuts() {
  return (
    <SettingsPageShell title="快捷键" desc="tianxuan 桌面端的全局键盘快捷键。⌘ = Mac Command, Ctrl = Windows/Linux。">
      <SettingsSection title={`${SHORTCUTS.length} 个快捷键`}>
        <div className="overflow-hidden rounded-lg border border-border-soft">
          <table className="w-full text-[12px]">
            <thead>
              <tr className="bg-bg-soft text-left">
                <th className="px-3 py-2 font-medium text-fg-dim w-[180px]">快捷键</th>
                <th className="px-3 py-2 font-medium text-fg-dim">操作</th>
                <th className="px-3 py-2 font-medium text-fg-dim w-[100px]">作用域</th>
              </tr>
            </thead>
            <tbody>
              {SHORTCUTS.map((s, i) => (
                <tr key={i} className="border-t border-border-soft">
                  <td className="px-3 py-2 font-mono text-fg">{s.keys}</td>
                  <td className="px-3 py-2 text-fg">{s.action}</td>
                  <td className="px-3 py-2 text-fg-faint">{s.context}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </SettingsSection>
    </SettingsPageShell>
  );
}
