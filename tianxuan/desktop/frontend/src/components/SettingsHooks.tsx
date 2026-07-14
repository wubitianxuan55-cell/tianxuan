import { useEffect, useState } from "react";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";
import { Plus, Trash2, Download, Upload, Zap } from "lucide-react";
import { app } from "../lib/bridge";
import type { HookConfigView, HooksSettingsView } from "../lib/types";

const VALID_EVENTS = [
  "PermissionRequest", "PreToolUse", "PostToolUse",
  "UserPromptSubmit", "Stop", "PostLLMCall",
  "SessionStart", "SessionEnd", "SubagentStop",
  "Notification", "PreCompact",
];

export function SettingsHooks() {
  const [hooks, setHooks] = useState<Record<string, HookConfigView[]>>({});
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [jsonInput, setJsonInput] = useState("");
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  // Load hooks from backend on mount.
  useEffect(() => {
    app.HooksSettings().then((v: HooksSettingsView) => {
      setHooks(v.hooks || {});
      setLoaded(true);
    }).catch((e: any) => {
      setError(String(e));
      setLoaded(true);
    });
  }, []);

  const save = async (next: Record<string, HookConfigView[]>) => {
    setHooks(next);
    setBusy(true);
    try {
      await app.SaveHooksSettings(next);
      setError(null);
    } catch (e: any) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const addHook = (forEvent: string) => {
    const next = { ...hooks };
    if (!next[forEvent]) next[forEvent] = [];
    next[forEvent] = [...next[forEvent], { match: "", command: "", description: "", timeout: 0, cwd: "" }];
    save(next);
    setExpanded(forEvent + "-" + (next[forEvent].length - 1));
  };

  const removeHook = (eventName: string, idx: number) => {
    const next = { ...hooks };
    next[eventName] = next[eventName].filter((_, i) => i !== idx);
    if (next[eventName].length === 0) delete next[eventName];
    save(next);
  };

  const updateHook = (eventName: string, idx: number, field: keyof HookConfigView, value: string | number) => {
    const next = { ...hooks };
    next[eventName] = [...next[eventName]];
    next[eventName][idx] = { ...next[eventName][idx], [field]: value };
    setHooks(next); // optimistic UI update
  };

  const commitHook = () => {
    save(hooks);
  };

  const importJSON = () => {
    setJsonError(null);
    try {
      const parsed = JSON.parse(jsonInput);
      if (!parsed || typeof parsed !== "object") throw new Error("根元素必须是 JSON 对象");
      if (!parsed.hooks || typeof parsed.hooks !== "object") throw new Error('缺少顶层 "hooks" 字段');
      const out: Record<string, HookConfigView[]> = {};
      for (const [evt, configs] of Object.entries(parsed.hooks)) {
        if (!Array.isArray(configs)) throw new Error(`事件 "${evt}" 的值必须是数组`);
        out[evt] = (configs as any[]).map((c: any) => ({
          match: String(c.match || ""),
          command: String(c.command || ""),
          description: String(c.description || ""),
          timeout: Number(c.timeout || 0),
          cwd: String(c.cwd || ""),
        }));
      }
      save(out);
      setJsonInput("");
    } catch (e: any) {
      setJsonError(e.message || String(e));
    }
  };

  const exportJSON = () => {
    const text = JSON.stringify({ hooks }, null, 2);
    navigator.clipboard.writeText(text).catch(() => {
      // fallback: show in a textarea
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.left = "-9999px";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    });
  };

  if (!loaded) {
    return (
      <SettingsPageShell title={<span className="flex items-center gap-1.5"><Zap size={15} className="text-accent" />钩子</span>} desc="事件驱动的脚本钩子，在会话生命周期特定时刻自动执行。">
        <div className="text-fg-faint text-[13px] py-8 text-center">加载中…</div>
      </SettingsPageShell>
    );
  }

  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><Zap size={15} className="text-accent" />钩子</span>} desc="事件驱动的脚本钩子，在会话生命周期特定时刻自动执行。保存后立即生效。">
      {error && (
        <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-3">
          {error}
        </div>
      )}
      {busy && (
        <div className="text-accent text-[12px] mb-3">保存中…</div>
      )}

      {/* Per-event hook lists */}
      {VALID_EVENTS.map(event => {
        const list = hooks[event] || [];
        if (list.length === 0 && expanded !== event) return null;
        return (
          <SettingsSection key={event} title={`${event} (${list.length})`}>
            {list.map((hook, idx) => {
              const key = event + "-" + idx;
              const isExpanded = expanded === key;
              return (
                <div key={key} className="bg-bg border border-border rounded-lg p-3 mb-2">
                  {/* Preview row */}
                  <div
                    className="flex items-center justify-between cursor-pointer"
                    onClick={() => setExpanded(isExpanded ? null : key)}
                  >
                    <div className="flex-1 min-w-0">
                      <span className="text-fg text-[12px] font-medium block truncate">
                        {hook.command || <span className="text-fg-faint italic">无命令</span>}
                      </span>
                      {hook.description && (
                        <span className="text-fg-faint text-[11px] block truncate">{hook.description}</span>
                      )}
                    </div>
                    <div className="flex items-center gap-1 ml-2">
                      <span className="text-fg-faint text-[10px]">{isExpanded ? "收起" : "展开"}</span>
                      <button
                        className="p-1 text-fg-faint hover:text-red-400 cursor-pointer bg-transparent border-0 rounded"
                        onClick={(e) => { e.stopPropagation(); removeHook(event, idx); }}
                      >
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </div>

                  {/* Expanded editor */}
                  {isExpanded && (
                    <div className="mt-3 pt-3 border-t border-border-soft space-y-2.5">
                      <SettingsField label="匹配" hint="正则匹配工具名（PreToolUse/PostToolUse），* = 全部">
                        <input
                          className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
                          placeholder="例如：read_file|edit_file"
                          value={hook.match}
                          onChange={e => updateHook(event, idx, "match", e.target.value)}
                          onBlur={() => commitHook()}
                        />
                      </SettingsField>

                      <SettingsField label="命令" hint="执行的 shell 命令，exit 0=pass, exit 2=block">
                        <textarea
                          className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[60px]"
                          placeholder='echo "hello world"'
                          value={hook.command}
                          onChange={e => updateHook(event, idx, "command", e.target.value)}
                          onBlur={() => commitHook()}
                          rows={3}
                        />
                      </SettingsField>

                      <div className="grid grid-cols-2 gap-2.5">
                        <SettingsField label="描述" hint="可读标签">
                          <input
                            className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
                            placeholder="可选"
                            value={hook.description}
                            onChange={e => updateHook(event, idx, "description", e.target.value)}
                            onBlur={() => commitHook()}
                          />
                        </SettingsField>

                        <SettingsField label="超时(ms)" hint="毫秒，0=默认(5000/30000)">
                          <input
                            type="number" min="0" step="1000"
                            className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
                            placeholder="0"
                            value={hook.timeout || ""}
                            onChange={e => updateHook(event, idx, "timeout", Number(e.target.value))}
                            onBlur={() => commitHook()}
                          />
                        </SettingsField>
                      </div>

                      <SettingsField label="工作目录" hint="覆盖命令执行目录，空=继承当前工作目录">
                        <input
                          className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
                          placeholder="继承"
                          value={hook.cwd}
                          onChange={e => updateHook(event, idx, "cwd", e.target.value)}
                          onBlur={() => commitHook()}
                        />
                      </SettingsField>
                    </div>
                  )}
                </div>
              );
            })}
            <button
              className="flex items-center gap-1 text-[12px] text-accent hover:text-accent/80 cursor-pointer bg-transparent border border-dashed border-accent/30 rounded-md px-3 py-1.5 mt-1"
              onClick={() => addHook(event)}
            >
              <Plus size={13} /> 添加钩子
            </button>
          </SettingsSection>
        );
      })}

      {/* Add new event type */}
      <SettingsSection title="添加事件类型">
        <div className="flex flex-wrap gap-1.5">
          {VALID_EVENTS.filter(e => !hooks[e] || hooks[e].length === 0).map(event => (
            <button
              key={event}
              className="text-[11px] bg-bg border border-border-soft rounded-md px-2.5 py-1 cursor-pointer hover:border-accent/50 hover:text-accent text-fg-dim"
              onClick={() => { addHook(event); }}
            >
              <Plus size={10} className="inline mr-1" />
              {event}
            </button>
          ))}
          {VALID_EVENTS.every(e => hooks[e] && hooks[e].length > 0) && (
            <span className="text-fg-faint text-[11px] py-1">所有事件类型都已配置</span>
          )}
        </div>
      </SettingsSection>

      {/* JSON Import/Export */}
      <SettingsSection title="JSON 导入/导出">
        <div className="flex gap-2 mb-2">
          <button
            className="flex items-center gap-1 text-[12px] bg-bg border border-border-soft rounded-md px-3 py-1.5 cursor-pointer hover:border-accent/50 text-fg-dim"
            onClick={exportJSON}
          >
            <Download size={13} /> 导出 JSON
          </button>
        </div>
        <div className="flex flex-col gap-1.5">
          <textarea
            className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[80px]"
            placeholder='粘贴 settings.json 内容…'
            value={jsonInput}
            onChange={e => { setJsonInput(e.target.value); setJsonError(null); }}
            rows={5}
          />
          {jsonError && (
            <div className="text-red-400 text-[11px]">{jsonError}</div>
          )}
          <button
            className="flex items-center gap-1 text-[12px] bg-accent text-white border-0 rounded-md px-3 py-1.5 cursor-pointer self-start"
            onClick={importJSON}
          >
            <Upload size={13} /> 导入 JSON
          </button>
        </div>
      </SettingsSection>

      {/* Event reference */}
      <SettingsSection title="可用事件 (11)">
        <div className="grid grid-cols-3 gap-1.5 text-[11px]">
          {VALID_EVENTS.map(e => (
            <div key={e} className={`bg-bg rounded px-2 py-1 ${hooks[e]?.length ? "text-accent" : "text-fg-dim"}`}>
              {e}
              {hooks[e]?.length ? <span className="ml-1 text-fg-faint">({hooks[e].length})</span> : null}
            </div>
          ))}
        </div>
      </SettingsSection>
    </SettingsPageShell>
  );
}
