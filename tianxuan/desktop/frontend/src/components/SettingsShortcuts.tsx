import { useEffect, useRef, useState } from "react";
import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";
import { RefreshCw } from "lucide-react";

interface ShortcutItem { id: string; keys: string; action: string; context: string; defaultKeys: string }

const SHORTCUTS: ShortcutItem[] = [
  { id: "new-session", keys: "n", action: "新建会话", context: "非运行中", defaultKeys: "n" },
  { id: "command-palette", keys: "k", action: "打开命令面板", context: "全局", defaultKeys: "k" },
  { id: "settings", keys: ",", action: "打开设置", context: "全局", defaultKeys: "," },
  { id: "memory", keys: "shift+m", action: "打开记忆面板", context: "全局", defaultKeys: "shift+m" },
  { id: "history", keys: "shift+h", action: "打开历史记录", context: "全局", defaultKeys: "shift+h" },
  { id: "sidebar", keys: "b", action: "切换侧边栏", context: "全局", defaultKeys: "b" },
  { id: "workspace-panel", keys: "j", action: "切换工作区面板", context: "全局", defaultKeys: "j" },
  { id: "escape", keys: "esc", action: "关闭面板 / 取消操作", context: "有面板打开时", defaultKeys: "esc" },
  { id: "send", keys: "enter", action: "发送消息", context: "输入框聚焦", defaultKeys: "enter" },
  { id: "newline", keys: "shift+enter", action: "换行", context: "输入框聚焦", defaultKeys: "shift+enter" },
];

// Load/save custom shortcuts from localStorage
function loadCustomShortcuts(): Record<string, string> {
  try { return JSON.parse(localStorage.getItem("tianxuan.shortcuts") || "{}"); } catch { return {}; }
}
function saveCustomShortcuts(map: Record<string, string>) {
  try { localStorage.setItem("tianxuan.shortcuts", JSON.stringify(map)); } catch {}
}
function resetCustomShortcuts() {
  try { localStorage.removeItem("tianxuan.shortcuts"); } catch {}
}

function comboLabel(combo: string): string {
  return combo
    .split("+")
    .map((k) => {
      if (k === "shift") return "⇧";
      if (k === "ctrl") return "⌃";
      if (k === "meta") return "⌘";
      if (k === "alt") return "⌥";
      if (k === "escape") return "Esc";
      if (k === "enter") return "↵";
      return k.charAt(0).toUpperCase() + k.slice(1);
    })
    .join("+");
}

export function SettingsShortcuts() {
  
  const [recording, setRecording] = useState<string | null>(null);
  const [conflict, setConflict] = useState<string | null>(null);
  const [customMap, setCustomMap] = useState<Record<string, string>>(() => loadCustomShortcuts());
  const recordingRef = useRef<string | null>(null);

  useEffect(() => {
    if (!recording) return;
    recordingRef.current = recording;
    const handler = (e: KeyboardEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const id = recordingRef.current;
      if (!id) return;
      const parts: string[] = [];
      if (e.ctrlKey) parts.push("ctrl");
      if (e.metaKey) parts.push("meta");
      if (e.altKey) parts.push("alt");
      if (e.shiftKey) parts.push("shift");
      const key = e.key.toLowerCase();
      if (key === "control" || key === "shift" || key === "alt" || key === "meta") {
        return; // don't record modifier-only
      }
      parts.push(key === " " ? "space" : key);
      const combo = parts.join("+");

      // Check conflict
      for (const [otherId, otherCombo] of Object.entries(customMap)) {
        if (otherId !== id && otherCombo === combo) {
          const other = SHORTCUTS.find((s) => s.id === otherId);
          setConflict(`快捷键 "${comboLabel(combo)}" 已被 "${other?.action || otherId}" 使用`);
          return;
        }
      }
      // Also check defaults
      for (const sc of SHORTCUTS) {
        if (sc.id !== id && sc.defaultKeys === combo) {
          setConflict(`快捷键 "${comboLabel(combo)}" 已被 "${sc.action}" 使用`);
          return;
        }
      }

      const next = { ...customMap, [id]: combo };
      setCustomMap(next);
      saveCustomShortcuts(next);
      setConflict(null);
      setRecording(null);
      // revision bump
    };
    document.addEventListener("keydown", handler, true);
    return () => document.removeEventListener("keydown", handler, true);
  }, [recording, customMap]);

  const resolved = (id: string): string => customMap[id] || SHORTCUTS.find((s) => s.id === id)?.defaultKeys || "";

  return (
    <SettingsPageShell title="快捷键" desc="tianxuan 桌面端的全局键盘快捷键。点击快捷键按钮进入录制模式，按新的组合键替换。⌘ = Mac Command, Ctrl = Windows/Linux。">
      {conflict && (
        <div className="px-3 py-2 mb-3 text-[12px] text-red-500 bg-red-500/10 border border-red-500/30 rounded-md" role="alert">
          {conflict}
        </div>
      )}
      <SettingsSection title={`${SHORTCUTS.length} 个快捷键`}>
        <div className="overflow-hidden rounded-lg border border-border-soft">
          <table className="w-full text-[12px]">
            <thead>
              <tr className="bg-bg-soft text-left">
                <th className="px-3 py-2 font-medium text-fg-dim w-[160px]">快捷键</th>
                <th className="px-3 py-2 font-medium text-fg-dim">操作</th>
                <th className="px-3 py-2 font-medium text-fg-dim w-[90px]">作用域</th>
                <th className="px-3 py-2 font-medium text-fg-dim w-[60px]">
                  <button
                    type="button"
                    className="bg-transparent border-0 text-fg-faint cursor-pointer hover:text-fg"
                    title="重置全部"
                    aria-label="重置全部快捷键"
                    onClick={() => {
                      resetCustomShortcuts();
                      setCustomMap({});
                      setConflict(null);
                      setRecording(null);
                      // revision bump
                    }}
                  >
                    <RefreshCw size={13} />
                  </button>
                </th>
              </tr>
            </thead>
            <tbody>
              {SHORTCUTS.map((sc) => {
                const combo = resolved(sc.id);
                const isCustom = combo !== sc.defaultKeys;
                const isRecording = recording === sc.id;
                return (
                  <tr key={sc.id} className="border-t border-border-soft">
                    <td className="px-3 py-2">
                      <button
                        type="button"
                        className={`font-mono text-fg px-2 py-0.5 rounded border text-[11px] cursor-pointer transition-colors ${
                          isRecording
                            ? "bg-accent/20 border-accent text-accent animate-pulse"
                            : "bg-bg-soft border-border-soft hover:border-fg-faint"
                        }`}
                        disabled={false}
                        aria-label={isRecording ? "录制中，按新组合键..." : comboLabel(combo)}
                        onClick={() => { setRecording(sc.id); setConflict(null); }}
                      >
                        {isRecording ? "按下按键..." : comboLabel(combo)}
                      </button>
                    </td>
                    <td className="px-3 py-2 text-fg">{sc.action}</td>
                    <td className="px-3 py-2 text-fg-faint">{sc.context}</td>
                    <td className="px-3 py-2">
                      <button
                        type="button"
                        className={`text-[11px] bg-transparent border-0 cursor-pointer ${
                          isCustom ? "text-fg-dim hover:text-fg" : "text-fg-faint"
                        }`}
                        disabled={!isCustom}
                        onClick={() => {
                          const next = { ...customMap };
                          delete next[sc.id];
                          setCustomMap(next);
                          saveCustomShortcuts(next);
                          // revision bump
                        }}
                      >
                        重置
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </SettingsSection>
    </SettingsPageShell>
  );
}
