import { useState } from "react";
import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";
import { Plus, Trash2 } from "lucide-react";

type HookConfig = {
  event: string;
  match: string;
  command: string;
  description: string;
  timeout: number;
};

const VALID_EVENTS = [
  "SessionStart",
  "SessionEnd",
  "PreToolUse",
  "PostToolUse",
  "Stop",
  "PreCompact",
  "PostLLMCall",
  "Notification",
  "SubagentStop",
];

export function SettingsHooks() {
  const [hooks, setHooks] = useState<HookConfig[]>(() => {
    try {
      const saved = localStorage.getItem("tianxuan.hooks");
      return saved ? JSON.parse(saved) : [];
    } catch { return []; }
  });
  const [jsonInput, setJsonInput] = useState("");
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  const saveHooks = (next: HookConfig[]) => {
    setHooks(next);
    try { localStorage.setItem("tianxuan.hooks", JSON.stringify(next)); } catch {}
  };

  const addHook = () => {
    const newHook: HookConfig = { event: "PreToolUse", match: "", command: "", description: "", timeout: 0 };
    saveHooks([...hooks, newHook]);
    setExpanded(newHook.event + "-" + hooks.length);
  };

  const removeHook = (i: number) => {
    saveHooks(hooks.filter((_, idx) => idx !== i));
  };

  const updateHook = (i: number, field: keyof HookConfig, value: string | number) => {
    const next = [...hooks];
    next[i] = { ...next[i], [field]: value };
    saveHooks(next);
  };

  const importJSON = () => {
    setJsonError(null);
    try {
      const parsed = JSON.parse(jsonInput);
      const arr = Array.isArray(parsed) ? parsed : [parsed];
      const valid = arr.map((item: any) => ({
        event: typeof item.event === "string" && VALID_EVENTS.includes(item.event) ? item.event : "PreToolUse",
        match: typeof item.match === "string" ? item.match : "",
        command: typeof item.command === "string" ? item.command : "",
        description: typeof item.description === "string" ? item.description : "",
        timeout: typeof item.timeout === "number" && item.timeout > 0 ? Math.floor(item.timeout) : 0,
      }));
      saveHooks(valid);
      setJsonInput("");
    } catch (e) {
      setJsonError("JSON 解析失败: " + String((e as Error).message));
    }
  };

  const exportJSON = () => {
    const json = JSON.stringify(hooks, null, 2);
    navigator.clipboard.writeText(json).catch(() => {});
    setJsonInput(json);
  };

  return (
    <SettingsPageShell title="钩子" desc="事件驱动的脚本钩子，在会话生命周期特定时刻自动执行。当前编辑保存在本地，完整支持需后端持久化。">
      {/* Hook list */}
      <SettingsSection title={`${hooks.length} 个钩子`}>
        {hooks.length === 0 && (
          <div className="text-[12px] text-fg-faint">尚未配置钩子。点击下方按钮添加。</div>
        )}
        {hooks.map((hook, i) => (
          <div key={`${hook.event}-${i}`} className="border border-border-soft rounded-lg overflow-hidden">
            <button
              type="button"
              className="w-full flex items-center gap-2 px-3 py-2 text-[12px] text-left bg-transparent border-0 cursor-pointer hover:bg-bg-soft"
              onClick={() => setExpanded(expanded === `${hook.event}-${i}` ? null : `${hook.event}-${i}`)}
            >
              <span className="font-mono text-fg">{hook.event}</span>
              {hook.command && <span className="text-fg-faint truncate flex-1">{hook.command}</span>}
              <button
                type="button"
                className="text-fg-faint hover:text-red-500 bg-transparent border-0 cursor-pointer p-0.5"
                onClick={(e) => { e.stopPropagation(); removeHook(i); }}
                aria-label="删除钩子"
              >
                <Trash2 size={12} />
              </button>
            </button>
            {expanded === `${hook.event}-${i}` && (
              <div className="px-3 pb-3 space-y-2 border-t border-border-soft">
                <div className="flex items-center gap-2 mt-2">
                  <label className="text-[11px] text-fg-dim w-16">事件</label>
                  <select
                    className="flex-1 bg-bg-soft border border-border-soft rounded text-fg text-[12px] px-2 py-1 outline-none"
                    value={hook.event}
                    onChange={(e) => updateHook(i, "event", e.target.value)}
                  >
                    {VALID_EVENTS.map((ev) => (
                      <option key={ev} value={ev}>{ev}</option>
                    ))}
                  </select>
                </div>
                <div className="flex items-center gap-2">
                  <label className="text-[11px] text-fg-dim w-16">匹配</label>
                  <input
                    className="flex-1 bg-bg-soft border border-border-soft rounded text-fg text-[12px] px-2 py-1 outline-none"
                    placeholder="文件匹配模式 (glob)"
                    value={hook.match}
                    onChange={(e) => updateHook(i, "match", e.target.value)}
                  />
                </div>
                <div className="flex items-center gap-2">
                  <label className="text-[11px] text-fg-dim w-16">命令</label>
                  <input
                    className="flex-1 bg-bg-soft border border-border-soft rounded text-fg text-[12px] px-2 py-1 outline-none"
                    placeholder="shell 命令"
                    value={hook.command}
                    onChange={(e) => updateHook(i, "command", e.target.value)}
                  />
                </div>
                <div className="flex items-center gap-2">
                  <label className="text-[11px] text-fg-dim w-16">描述</label>
                  <input
                    className="flex-1 bg-bg-soft border border-border-soft rounded text-fg text-[12px] px-2 py-1 outline-none"
                    placeholder="可选描述"
                    value={hook.description}
                    onChange={(e) => updateHook(i, "description", e.target.value)}
                  />
                </div>
                <div className="flex items-center gap-2">
                  <label className="text-[11px] text-fg-dim w-16">超时(s)</label>
                  <input
                    className="w-20 bg-bg-soft border border-border-soft rounded text-fg text-[12px] px-2 py-1 outline-none"
                    type="number"
                    min={0}
                    value={hook.timeout}
                    onChange={(e) => updateHook(i, "timeout", Math.max(0, Number(e.target.value)))}
                  />
                </div>
              </div>
            )}
          </div>
        ))}

        <button
          type="button"
          className="flex items-center gap-1.5 text-[12px] text-fg-dim bg-transparent border border-border-soft rounded px-3 py-1.5 cursor-pointer hover:text-fg hover:border-fg-faint mt-2"
          onClick={addHook}
        >
          <Plus size={13} /> 添加钩子
        </button>
      </SettingsSection>

      {/* JSON import/export */}
      <SettingsSection title="JSON 导入/导出">
        <div className="flex gap-2 mb-2">
          <button
            type="button"
            className="text-[11px] text-fg-dim bg-transparent border border-border-soft rounded px-2.5 py-1 cursor-pointer hover:text-fg"
            onClick={exportJSON}
          >
            导出 JSON
          </button>
          <button
            type="button"
            className="text-[11px] text-fg-dim bg-transparent border border-border-soft rounded px-2.5 py-1 cursor-pointer hover:text-fg"
            onClick={importJSON}
          >
            导入 JSON
          </button>
        </div>
        {jsonInput && (
          <textarea
            className="w-full h-24 bg-bg-soft border border-border-soft rounded text-fg text-[11px] font-mono px-2 py-1.5 outline-none resize-y"
            value={jsonInput}
            onChange={(e) => setJsonInput(e.target.value)}
            placeholder='[{"event":"SessionStart","command":"echo hello"}]'
          />
        )}
        {jsonError && <div className="text-[11px] text-red-500 mt-1">{jsonError}</div>}
      </SettingsSection>

      {/* Event reference */}
      <SettingsSection title="可用事件">
        <div className="grid grid-cols-2 gap-1">
          {VALID_EVENTS.map((ev) => (
            <div key={ev} className="text-[11px] text-fg-faint font-mono">{ev}</div>
          ))}
        </div>
      </SettingsSection>
    </SettingsPageShell>
  );
}
