import { useState } from "react";
import { ChevronDown, ChevronRight, Globe, Palette, Wrench, Bot, BrainCircuit } from "lucide-react";
import type { SectionProps } from "./SettingsShared";
import { SettingsPageShell, SettingsSection, SettingsField, SegmentedButton } from "./SettingsPageShell";
import { app } from "../lib/bridge";
import { useI18n } from "../lib/i18n";

type SoundPref = "off" | "synth";

const STATUS_BAR_ITEMS = ["model", "workspace", "gitBranch", "cache", "tokens", "jobs", "balance"];

// ── CollapsibleSection: chevron-based expand/collapse ──

function CollapsibleSection(p: {
  label: string;
  summary: string;
  expanded: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className="mb-1">
      <button
        type="button"
        className="flex items-center gap-1.5 w-full text-left bg-transparent border-0 text-fg-dim text-[12px] font-medium py-1 cursor-pointer hover:text-fg transition-colors pt-0.5 border-t border-border-soft/40 mt-1"
        onClick={p.onToggle}
      >
        {p.expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        <span>{p.label}</span>
        {!p.expanded && (
          <span className="text-fg-faint text-[11px] font-normal truncate ml-1">· {p.summary}</span>
        )}
      </button>
      {p.expanded && <div className="pl-3 ml-1 border-l-2 border-border-soft bg-bg/40 rounded-r-md p-2.5 space-y-2">{p.children}</div>}
    </div>
  );
}

// ── main ──

export function SettingsGeneral({ s, busy: _busy, apply }: SectionProps) {
  const { t, pref, setPref } = useI18n();

  const [depth, setDepth] = useState(s.agent.maxSubagentDepth);
  const [plannerSteps, setPlannerSteps] = useState(s.agent.plannerMaxSteps || 0);
  const [reasoningLang, setReasoningLang] = useState(s.agent.reasoningLanguage || "auto");
  const [autoPlan, setAutoPlan] = useState(s.agent.autoPlan || "off");
  const [outputStyle, setOutputStyle] = useState(s.agent.outputStyle || "");
  const [coldResume, setColdResume] = useState(s.agent.coldResumePrune);
  const [memCompiler, setMemCompiler] = useState(s.agent.memoryCompilerEnabled || false);

  const desk = s.desktop;
  const [layoutStyle, setLayoutStyle] = useState(desk.layoutStyle || "classic");
  const [closeBehavior, setCloseBehavior] = useState(desk.closeBehavior || "quit");
  const [displayMode, setDisplayMode] = useState(() => {
    try { return localStorage.getItem("tianxuan.displayMode") || "standard"; } catch { return "standard"; }
  });
  const [statusBarStyle, setStatusBarStyle] = useState(desk.statusBarStyle || "icon");
  const [statusBarItems, setStatusBarItems] = useState<string[]>(desk.statusBarItems?.length ? desk.statusBarItems : STATUS_BAR_ITEMS);
  const [statusBarExpanded, setStatusBarExpanded] = useState(false);

  const [toolApprovalMode, setToolApprovalMode] = useState(s.permissions.mode || "ask");
  const [shellPref, setShellPref] = useState(s.tools?.shell || "auto");
  const [bashTimeout, setBashTimeout] = useState(s.tools?.bashTimeoutSeconds ? String(s.tools.bashTimeoutSeconds) : "");
  const [mcpTimeout, setMcpTimeout] = useState(s.tools?.mcpCallTimeoutSeconds ? String(s.tools.mcpCallTimeoutSeconds) : "");

  const [soundPref, setSoundPref] = useState<SoundPref>(() => {
    try { return (localStorage.getItem("tianxuan.soundSuccess") as SoundPref) || "synth"; } catch { return "synth"; }
  });
  const [attentionPref, setAttentionPref] = useState<SoundPref>(() => {
    try { return (localStorage.getItem("tianxuan.soundAttention") as SoundPref) || "synth"; } catch { return "synth"; }
  });
  const [soundExpanded, setSoundExpanded] = useState(false);

  const updateLanguage = (lang: "" | "en" | "zh" | "zh-TW") => {
    setPref(lang);
    void apply(() => app.SetDesktopLanguage(lang));
  };

  const statusBarSummary = statusBarItems.length >= STATUS_BAR_ITEMS.length
    ? "全部"
    : statusBarItems.slice(0, 3).join(", ") + (statusBarItems.length > 3 ? `… +${statusBarItems.length - 3}` : "");

  const soundSummary = soundPref === "off" && attentionPref === "off"
    ? "全部关闭"
    : `${soundPref === "synth" ? "成功提示" : ""}${soundPref === "synth" && attentionPref === "synth" ? " · " : ""}${attentionPref === "synth" ? "注意提示" : ""}`;

  return (
    <SettingsPageShell title="通用" desc="界面语言、工具参数、智能体行为与记忆管理等基础设置。">
      <div className="space-y-5">
      {/* ── 语言 ── */}
      <SettingsSection title={
        <span className="flex items-center gap-1.5"><Globe size={14} className="text-accent" />语言</span>
      }>
        <SettingsField label="界面语言" hint="桌面界面的显示语言，自动 = 跟随系统。">
          <SegmentedButton
            options={[
              { value: "" as const, label: t("settings.langAuto") },
              { value: "zh" as const, label: "简体中文" },
              { value: "zh-TW" as const, label: "繁體中文" },
              { value: "en" as const, label: "English" },
            ]}
            value={pref}
            onChange={updateLanguage}
          />
        </SettingsField>
      </SettingsSection>

      {/* ── 外观与布局 ── */}
      <SettingsSection title={
        <span className="flex items-center gap-1.5"><Palette size={14} className="text-accent" />外观与布局</span>
      }>
        <SettingsField label="布局风格" hint="桌面窗口的整体布局样式。">
          <SegmentedButton
            options={[
              { value: "classic", label: "经典" },
              { value: "workbench", label: "工作台" },
              { value: "creation", label: "创作" },
            ]}
            value={layoutStyle}
            onChange={(v) => { setLayoutStyle(v); void apply(() => app.SetDesktopLayoutStyle(v)); }}
          />
        </SettingsField>
        <SettingsField label="关闭行为" hint="点击关闭按钮时触发的操作。">
          <SegmentedButton
            options={[
              { value: "quit", label: "退出" },
              { value: "background", label: "最小化到托盘" },
            ]}
            value={closeBehavior}
            onChange={(v) => { setCloseBehavior(v); void apply(() => app.SetDesktopCloseBehavior(v)); }}
          />
        </SettingsField>
        <SettingsField label="显示模式" hint="对话区域的紧凑程度。">
          <SegmentedButton
            options={[
              { value: "standard", label: "标准" },
              { value: "compact", label: "紧凑" },
            ]}
            value={displayMode}
            onChange={(v) => {
              setDisplayMode(v);
              try { localStorage.setItem("tianxuan.displayMode", v); } catch {}
              void apply(() => app.SetDesktopDisplayMode(v));
            }}
          />
        </SettingsField>
        <SettingsField label="状态栏样式" hint="状态栏项目的呈现方式。">
          <SegmentedButton
            options={[
              { value: "icon", label: "图标" },
              { value: "text", label: "文字" },
            ]}
            value={statusBarStyle}
            onChange={(v) => { setStatusBarStyle(v); void apply(() => app.SetStatusBarStyle(v)); }}
          />
        </SettingsField>

        {/* status bar — collapsible */}
        <CollapsibleSection
          label="状态栏项目"
          summary={statusBarSummary}
          expanded={statusBarExpanded}
          onToggle={() => setStatusBarExpanded((v) => !v)}
        >
          <div className="space-y-1">
            {STATUS_BAR_ITEMS.map((id) => {
              const checked = statusBarItems.includes(id);
              return (
                <label key={id} className="flex items-center gap-2 text-[12px] text-fg-dim cursor-pointer">
                  <input
                    type="checkbox"
                    checked={checked}
                    disabled={_busy}
                    onChange={() => {
                      const next = checked
                        ? statusBarItems.filter((x) => x !== id)
                        : [...statusBarItems, id];
                      setStatusBarItems(next);
                      void apply(() => app.SetStatusBarItems(next));
                    }}
                  />
                  <span>{id}</span>
                </label>
              );
            })}
          </div>
        </CollapsibleSection>

        {/* sound — collapsible */}
        <CollapsibleSection
          label="声音反馈"
          summary={soundSummary}
          expanded={soundExpanded}
          onToggle={() => setSoundExpanded((v) => !v)}
        >
          <div className="flex items-center gap-3">
            <span className="text-[12px] text-fg-dim w-20">成功提示</span>
            <SegmentedButton
              options={[
                { value: "off", label: "关闭" },
                { value: "synth", label: "合成音" },
              ]}
              value={soundPref}
              onChange={(v) => { setSoundPref(v as SoundPref); try { localStorage.setItem("tianxuan.soundSuccess", v); } catch {} }}
            />
          </div>
          <div className="flex items-center gap-3">
            <span className="text-[12px] text-fg-dim w-20">注意提示</span>
            <SegmentedButton
              options={[
                { value: "off", label: "关闭" },
                { value: "synth", label: "合成音" },
              ]}
              value={attentionPref}
              onChange={(v) => { setAttentionPref(v as SoundPref); try { localStorage.setItem("tianxuan.soundAttention", v); } catch {} }}
            />
          </div>
        </CollapsibleSection>
      </SettingsSection>

      {/* ── 工具 ── */}
      <SettingsSection title={
        <span className="flex items-center gap-1.5"><Wrench size={14} className="text-accent" />工具</span>
      }>
        <SettingsField label="工具审批模式" hint="工具写入前是否需要确认。">
          <SegmentedButton
            options={[
              { value: "ask", label: "询问" },
              { value: "auto", label: "自动" },
              { value: "yolo", label: "YOLO" },
            ]}
            value={toolApprovalMode}
            onChange={(v) => { setToolApprovalMode(v); void apply(() => app.SetPermissionMode(v)); }}
          />
        </SettingsField>
        <SettingsField label="Shell 偏好" hint="执行命令时使用的 Shell 解释器。">
          <SegmentedButton
            options={[
              { value: "auto", label: "自动" },
              { value: "bash", label: "Bash" },
              { value: "powershell", label: "PowerShell" },
            ]}
            value={shellPref}
            onChange={(v) => { setShellPref(v); void apply(() => app.SetShellPreference(v)); }}
          />
        </SettingsField>
        <SettingsField label="Bash 超时" hint="命令执行超时秒数，0 = 默认 (120s)。">
          <input
            type="number" min="0" max="600" step="10"
            className="w-24 bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            placeholder="默认"
            value={bashTimeout}
            onChange={(e) => {
              const v = e.target.value;
              setBashTimeout(v);
              const n = parseInt(v, 10);
              if (!isNaN(n) && n >= 0) void apply(() => app.SetBashTimeoutSeconds(n));
            }}
          />
        </SettingsField>
        <SettingsField label="MCP 超时" hint="MCP 调用超时秒数，0 = 默认 (300s)。">
          <input
            type="number" min="0" max="3600" step="30"
            className="w-24 bg-bg border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent"
            placeholder="默认"
            value={mcpTimeout}
            onChange={(e) => {
              const v = e.target.value;
              setMcpTimeout(v);
              const n = parseInt(v, 10);
              if (!isNaN(n) && n >= 0) void apply(() => app.SetMCPCallTimeoutSeconds(n));
            }}
          />
        </SettingsField>
      </SettingsSection>

      {/* ── 智能体 ── */}
      <SettingsSection title={
        <span className="flex items-center gap-1.5"><Bot size={14} className="text-accent" />智能体</span>
      }>
        <SettingsField label="自动规划" hint="多步任务自动启用规划模式。off=手动 / ask=询问 / on=自动。">
          <SegmentedButton
            options={[
              { value: "off", label: "关闭" },
              { value: "ask", label: "询问" },
              { value: "on", label: "开启" },
            ]}
            value={autoPlan}
            onChange={(v) => { setAutoPlan(v); void apply(() => app.SetAutoPlan(v)); }}
          />
        </SettingsField>
        <SettingsField label="规划器最大步数" hint="规划阶段工具调用轮数上限。0 = 不限。">
          <SegmentedButton
            options={[
              { value: "6", label: "6" },
              { value: "12", label: "12" },
              { value: "25", label: "25" },
              { value: "0", label: "∞" },
            ]}
            value={String(plannerSteps)}
            onChange={(v) => {
              const n = Number(v);
              setPlannerSteps(n);
              void apply(() => app.SetPlannerMaxSteps(n));
            }}
          />
        </SettingsField>
        <SettingsField label="递归深度限制" hint="限制子代理嵌套层数。0 = 不限。">
          <SegmentedButton
            options={[
              { value: "0", label: "不限" },
              { value: "1", label: "1" },
              { value: "2", label: "2" },
              { value: "3", label: "3" },
            ]}
            value={String(depth)}
            onChange={(v) => {
              const n = Number(v);
              setDepth(n);
              void apply(() => app.SetMaxSubagentDepth(n));
            }}
          />
        </SettingsField>
        <SettingsField label="推理语言" hint="控制模型思考文本的语言偏好。">
          <SegmentedButton
            options={[
              { value: "auto", label: "自动" },
              { value: "zh", label: "中文" },
              { value: "en", label: "English" },
            ]}
            value={reasoningLang}
            onChange={(v) => { setReasoningLang(v); void apply(() => app.SetReasoningLanguage(v)); }}
          />
        </SettingsField>
        <SettingsField label="输出风格" hint="影响智能体回复的语气和详细程度。">
          <SegmentedButton
            options={[
              { value: "", label: "默认" },
              { value: "concise", label: "简洁" },
              { value: "explanatory", label: "详细" },
            ]}
            value={outputStyle}
            onChange={(v) => { setOutputStyle(v); void apply(() => app.SetOutputStyle(v)); }}
          />
        </SettingsField>
      </SettingsSection>

      {/* ── 记忆与上下文 ── */}
      <SettingsSection title={
        <span className="flex items-center gap-1.5"><BrainCircuit size={14} className="text-accent" />记忆与上下文</span>
      }>
        <SettingsField label="冷恢复修剪" hint="冷启动恢复时自动移除过期工具结果以节省上下文。">
          <SegmentedButton
            options={[
              { value: "false", label: "关闭" },
              { value: "true", label: "开启" },
            ]}
            value={String(coldResume)}
            onChange={(v) => {
              const on = v === "true";
              setColdResume(on);
              void apply(() => app.SetColdResumePrune(on));
            }}
          />
        </SettingsField>
        <SettingsField label="Memory 编译器" hint="启用 v5 执行记忆编译器，自动从历史推理中提炼持久记忆。">
          <SegmentedButton
            options={[
              { value: "false", label: "关闭" },
              { value: "true", label: "开启" },
            ]}
            value={String(memCompiler)}
            onChange={(v) => {
              const on = v === "true";
              setMemCompiler(on);
              void apply(() => app.SetMemoryCompilerEnabled(on));
            }}
          />
        </SettingsField>
      </SettingsSection>
    </div>
    </SettingsPageShell>
  );
}
