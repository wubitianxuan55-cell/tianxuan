import { useState, useEffect, useCallback } from "react";
import { Plus, Play, Trash2, Settings, ChevronDown, ChevronRight, CalendarDays } from "lucide-react";
import { app } from "../lib/bridge";
import type { ScheduleView, ResultView } from "../lib/types";

const DAY_NAMES = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];

function freqLabel(s: ScheduleView): string {
  switch (s.frequency) {
    case "hourly":
      return "每小时整点";
    case "daily":
      return `每天 ${s.time}`;
    case "weekly":
      return `每周${DAY_NAMES[s.dayOfWeek] || ""} ${s.time}`;
    default:
      return s.frequency;
  }
}

function statusDot(s: ScheduleView, results: ResultView[]): string {
  if (!s.enabled) return "⏸";
  if (results.length === 0) return "🔵";
  return results[results.length - 1].success ? "🟢" : "🟡";
}

function fmtTime(ts: number): string {
  const d = new Date(ts * 1000);
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export function SchedulePanel({ onClose }: { onClose: () => void }) {
  const [schedules, setSchedules] = useState<ScheduleView[]>([]);
  const [results, setResults] = useState<Record<string, ResultView[]>>({});
  const [expanded, setExpanded] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<ScheduleView | null>(null);
  const [form, setForm] = useState({
    name: "",
    prompt: "",
    frequency: "daily",
    time: "08:00",
    dayOfWeek: 1,
    workDir: "",
    envStr: "",
    scope: "global",
    enabled: true,
  });

  const load = useCallback(async () => {
    try {
      const s = await app.GetSchedules();
      setSchedules(s || []);
    } catch {
      /* not ready */
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const loadResults = useCallback(async (scheduleId: string) => {
    try {
      const r = await app.GetResults(scheduleId);
      setResults((p) => ({ ...p, [scheduleId]: r || [] }));
    } catch {
      /* */
    }
  }, []);

  const toggleExpand = (id: string) => {
    if (expanded === id) {
      setExpanded(null);
      return;
    }
    setExpanded(id);
    loadResults(id);
  };

  const handleToggle = async (id: string, enabled: boolean) => {
    await app.ToggleSchedule(id, enabled);
    load();
  };
  const handleRunNow = async (id: string) => {
    await app.RunScheduleNow(id);
    load();
    loadResults(id);
  };
  const handleDelete = async (id: string) => {
    await app.DeleteSchedule(id);
    load();
  };
  const handleSubmit = async () => {
    const env: Record<string, string> = {};
    if (form.envStr.trim()) {
      form.envStr.split("\n").forEach((line) => {
        const idx = line.indexOf("=");
        if (idx > 0) env[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
      });
    }
    const s: ScheduleView = {
      id: editing?.id || "",
      name: form.name,
      prompt: form.prompt,
      frequency: form.frequency,
      time: form.time,
      dayOfWeek: form.dayOfWeek,
      workDir: form.workDir,
      env,
      enabled: form.enabled,
      scope: form.scope,
      createdAt: editing?.createdAt || 0,
      lastRunAt: editing?.lastRunAt || 0,
    };
    if (editing) {
      await app.UpdateSchedule(s);
    } else {
      await app.CreateSchedule(s);
    }
    setFormOpen(false);
    setEditing(null);
    load();
  };

  const openEdit = (s: ScheduleView) => {
    setEditing(s);
    setForm({
      name: s.name,
      prompt: s.prompt,
      frequency: s.frequency,
      time: s.time,
      dayOfWeek: s.dayOfWeek,
      workDir: s.workDir,
      envStr: Object.entries(s.env || {})
        .map(([k, v]) => `${k}=${v}`)
        .join("\n"),
      scope: s.scope,
      enabled: s.enabled,
    });
    setFormOpen(true);
  };

  const globalScheds = schedules.filter((s) => s.scope === "global");
  const wsScheds = schedules.filter((s) => s.scope === "workspace");

  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-black/30" onClick={onClose}>
      <div
        className="w-[420px] h-full bg-bg border-l border-border-soft flex flex-col shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-border-soft">
          <div className="flex items-center gap-2 text-[15px] font-semibold text-fg">
            <CalendarDays size={17} /> 定时任务
          </div>
          <button
            className="flex items-center gap-1.5 rounded-full bg-accent text-accent-fg px-3 py-1.5 text-[13px] font-semibold hover:brightness-110 transition"
            onClick={() => {
              setEditing(null);
              setForm({
                name: "",
                prompt: "",
                frequency: "daily",
                time: "08:00",
                dayOfWeek: 1,
                workDir: "",
                envStr: "",
                scope: "global",
                enabled: true,
              });
              setFormOpen(true);
            }}
          >
            <Plus size={14} /> 新建
          </button>
        </div>
        {/* List */}
        <div className="flex-1 overflow-y-auto p-3 space-y-2">
          {globalScheds.length > 0 && (
            <>
              <div className="text-[11px] font-semibold uppercase text-fg-faint px-1 py-1">
                ● 全局任务
              </div>
              {globalScheds.map((s) => (
                <ScheduleCard
                  key={s.id}
                  s={s}
                  results={results[s.id] || []}
                  expanded={expanded === s.id}
                  onToggle={toggleExpand}
                  onEnable={(en) => handleToggle(s.id, en)}
                  onRun={() => handleRunNow(s.id)}
                  onEdit={() => openEdit(s)}
                  onDelete={() => handleDelete(s.id)}
                />
              ))}
            </>
          )}
          {wsScheds.length > 0 && (
            <>
              <div className="text-[11px] font-semibold uppercase text-fg-faint px-1 py-1">
                ● 当前工作区
              </div>
              {wsScheds.map((s) => (
                <ScheduleCard
                  key={s.id}
                  s={s}
                  results={results[s.id] || []}
                  expanded={expanded === s.id}
                  onToggle={toggleExpand}
                  onEnable={(en) => handleToggle(s.id, en)}
                  onRun={() => handleRunNow(s.id)}
                  onEdit={() => openEdit(s)}
                  onDelete={() => handleDelete(s.id)}
                />
              ))}
            </>
          )}
          {schedules.length === 0 && (
            <div className="text-center text-fg-faint text-[13px] py-8">
              暂无定时任务，点击"新建"创建
            </div>
          )}
        </div>
        {/* Form modal */}
        {formOpen && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
            onClick={() => setFormOpen(false)}
          >
            <div
              className="w-[440px] max-h-[90vh] overflow-y-auto bg-bg-elev border border-border rounded-xl shadow-lg p-5 space-y-4"
              onClick={(e) => e.stopPropagation()}
            >
              <h3 className="text-[15px] font-semibold text-fg">
                {editing ? "编辑任务" : "新建任务"}
              </h3>
              <label className="block text-[12px] text-fg-faint">
                名称{" "}
                <input
                  className="w-full mt-1 bg-bg border border-border-soft rounded-md px-2.5 py-2 text-[13px] text-fg outline-none focus:border-accent"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="如: 每日代码审查"
                />
              </label>
              <label className="block text-[12px] text-fg-faint">
                Prompt{" "}
                <textarea
                  className="w-full mt-1 bg-bg border border-border-soft rounded-md px-2.5 py-2 text-[13px] text-fg outline-none focus:border-accent resize-y min-h-[80px]"
                  value={form.prompt}
                  onChange={(e) => setForm({ ...form, prompt: e.target.value })}
                  placeholder="发给 AI 执行者的任务描述"
                />
              </label>
              <div className="flex gap-2">
                <label className="flex-1 text-[12px] text-fg-faint">
                  频率{" "}
                  <select
                    className="w-full mt-1 bg-bg border border-border-soft rounded-md px-2 py-2 text-[13px] text-fg outline-none"
                    value={form.frequency}
                    onChange={(e) => setForm({ ...form, frequency: e.target.value })}
                  >
                    <option value="hourly">每小时</option>
                    <option value="daily">每天</option>
                    <option value="weekly">每周</option>
                  </select>
                </label>
                <label className="w-28 text-[12px] text-fg-faint">
                  时间{" "}
                  <input
                    className="w-full mt-1 bg-bg border border-border-soft rounded-md px-2.5 py-2 text-[13px] text-fg outline-none"
                    value={form.time}
                    onChange={(e) => setForm({ ...form, time: e.target.value })}
                    placeholder="08:00"
                  />
                </label>
              </div>
              {form.frequency === "weekly" && (
                <label className="block text-[12px] text-fg-faint">
                  星期{" "}
                  <select
                    className="w-full mt-1 bg-bg border border-border-soft rounded-md px-2 py-2 text-[13px] text-fg outline-none"
                    value={form.dayOfWeek}
                    onChange={(e) => setForm({ ...form, dayOfWeek: +e.target.value })}
                  >
                    {DAY_NAMES.map((n, i) => (
                      <option key={i} value={i}>
                        {n}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <label className="block text-[12px] text-fg-faint">
                工作目录{" "}
                <input
                  className="w-full mt-1 bg-bg border border-border-soft rounded-md px-2.5 py-2 text-[13px] text-fg outline-none"
                  value={form.workDir}
                  onChange={(e) => setForm({ ...form, workDir: e.target.value })}
                  placeholder="/absolute/path/to/project"
                />
              </label>
              <label className="block text-[12px] text-fg-faint">
                环境变量 (每行 KEY=VALUE){" "}
                <textarea
                  className="w-full mt-1 bg-bg border border-border-soft rounded-md px-2.5 py-2 text-[13px] text-fg outline-none font-mono text-[11px]"
                  value={form.envStr}
                  onChange={(e) => setForm({ ...form, envStr: e.target.value })}
                  rows={3}
                  placeholder="NODE_ENV=production"
                />
              </label>
              <div className="flex gap-4">
                <label className="flex items-center gap-1.5 text-[13px] text-fg cursor-pointer">
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                  />{" "}
                  启用
                </label>
                <label className="text-[12px] text-fg-faint">
                  范围:{" "}
                  <select
                    className="ml-1 bg-bg border border-border-soft rounded px-1 py-0.5 text-[13px] text-fg"
                    value={form.scope}
                    onChange={(e) => setForm({ ...form, scope: e.target.value })}
                  >
                    <option value="global">全局</option>
                    <option value="workspace">当前工作区</option>
                  </select>
                </label>
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <button
                  className="px-3 py-1.5 text-[13px] rounded-md border border-border-soft text-fg-dim hover:bg-bg-soft transition"
                  onClick={() => {
                    setFormOpen(false);
                    setEditing(null);
                  }}
                >
                  取消
                </button>
                <button
                  className="px-3 py-1.5 text-[13px] rounded-full bg-accent text-accent-fg font-semibold hover:brightness-110 transition"
                  onClick={handleSubmit}
                >
                  {editing ? "保存" : "创建"}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function ScheduleCard({
  s,
  results,
  expanded,
  onToggle,
  onEnable,
  onRun,
  onEdit,
  onDelete,
}: {
  s: ScheduleView;
  results: ResultView[];
  expanded: boolean;
  onToggle: (id: string) => void;
  onEnable: (enabled: boolean) => void;
  onRun: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const last = results.length > 0 ? results[results.length - 1] : null;
  return (
    <div
      className={`rounded-xl border ${
        s.enabled ? "border-border-soft" : "border-border-soft/50"
      } bg-bg-elev overflow-hidden`}
    >
      <div
        className="flex items-center gap-2 px-3 py-2.5 cursor-pointer"
        onClick={() => onToggle(s.id)}
      >
        <button
          className="text-fg-faint"
          onClick={(e) => {
            e.stopPropagation();
            onEnable(!s.enabled);
          }}
          title={s.enabled ? "暂停" : "启用"}
        >
          <span className="text-[15px]">{statusDot(s, results)}</span>
        </button>
        <div className="flex-1 min-w-0">
          <div className="text-[13px] font-medium text-fg truncate">{s.name}</div>
          <div className="text-[11px] text-fg-faint">{freqLabel(s)}</div>
          <div className="text-[10.5px] text-fg-faint">
            {last
              ? `上次: ${fmtTime(last.executedAt)} ${last.success ? "✅" : "❌"} · ${last.summary.slice(0, 40)}`
              : "从未执行"}
          </div>
          {results.length > 0 && (
            <div className="text-[10px] text-fg-faint/70">
              共 {results.length} 次 · {results.filter((r) => !r.success).length} 失败
            </div>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
          <button className="p-1 text-fg-faint hover:text-fg transition" onClick={onRun} title="立即执行">
            <Play size={13} />
          </button>
          <button className="p-1 text-fg-faint hover:text-fg transition" onClick={onEdit} title="编辑">
            <Settings size={13} />
          </button>
          <button
            className="p-1 text-fg-faint hover:text-err transition"
            onClick={onDelete}
            title="删除"
          >
            <Trash2 size={13} />
          </button>
          {expanded ? (
            <ChevronDown size={13} className="text-fg-faint" />
          ) : (
            <ChevronRight size={13} className="text-fg-faint" />
          )}
        </div>
      </div>
      {expanded && (
        <div className="border-t border-border-soft px-3 py-2 space-y-1.5 max-h-[200px] overflow-y-auto">
          {results.length === 0 ? (
            <div className="text-[12px] text-fg-faint py-2 text-center">暂无执行记录</div>
          ) : (
            results.map((r) => (
              <div key={r.id} className="flex items-start gap-2 text-[12px]">
                <span className="text-[11px] shrink-0">{r.success ? "✅" : "❌"}</span>
                <div className="flex-1 min-w-0">
                  <span className="text-fg-faint">
                    {fmtTime(r.executedAt)} · {(r.duration / 1000).toFixed(1)}s
                  </span>
                  <div className="text-fg-dim truncate">{r.summary}</div>
                </div>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}
