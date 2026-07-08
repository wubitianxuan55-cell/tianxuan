import { FolderOpen, Bug, Search, FileText, MessageSquare, Clock, Zap, PenTool, TestTube, Wrench, Brain, Blocks, Cpu } from "lucide-react";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { sessionTitle } from "../lib/session";
import type { Meta, SessionMeta } from "../lib/types";

function formatTimeAgo(ms: number): string {
  const diff = Date.now() - ms;
  const min = Math.floor(diff / 60000);
  if (min < 1) return "刚刚";
  if (min < 60) return `${min}分钟前`;
  const hrs = Math.floor(min / 60);
  if (hrs < 24) return `${hrs}小时前`;
  return new Date(ms).toLocaleDateString([], { month: "short", day: "numeric" });
}

const QUICK_COMMANDS = [
  { icon: <Search size={14} />, label: "探索代码库", prompt: "explore this codebase — identify the key modules, their responsibilities, and how they connect" },
  { icon: <Bug size={14} />, label: "修复 Bug", prompt: "fix this bug: " },
  { icon: <PenTool size={14} />, label: "添加功能", prompt: "add a feature: " },
  { icon: <Zap size={14} />, label: "代码审查", prompt: "review my recent changes for issues" },
  { icon: <TestTube size={14} />, label: "写测试", prompt: "write tests for " },
  { icon: <FileText size={14} />, label: "写文档", prompt: "write documentation for " },
];

export function Welcome({
  onPrompt,
  cwd: _cwd,
  cwdName,
  sessions,
  onResumeSession,
  meta,
}: {
  onPrompt: (text: string) => void;
  cwd?: string;
  cwdName?: string;
  sessions?: SessionMeta[];
  onResumeSession?: (path: string) => Promise<void>;
  meta?: Meta;
}) {
  const t = useT();
  const compact = useCompact();

  const recentSessions = sessions?.filter(s => !s.current).slice(0, 3) ?? [];

  return (
    <div className="h-full flex flex-col items-center justify-center max-w-2xl mx-auto px-6 overflow-y-auto">
      {cwdName && (
        <div className={`inline-flex items-center gap-2 px-3 py-1.5 mb-5 rounded-full bg-accent-soft border border-accent/20 text-fg-dim ${compact ? "text-[11px]" : "text-[12px]"}`}>
          <FolderOpen size={compact ? 12 : 13} className="text-accent" />
          <span className="font-medium text-accent">{cwdName}</span>
          {meta?.label && <span className="text-fg-faint">· {meta.label}</span>}
        </div>
      )}

      <div className="mb-7">
        <div className="relative w-20 h-20 mx-auto flex items-center justify-center mb-[22px]">
          <span className="absolute inset-[-6px] rounded-2xl bg-accent/15 animate-pulse" />
          <span className="absolute inset-[-10px] rounded-2xl border-2 border-accent/25 animate-[spin_12s_linear_infinite]" />
          <span className="absolute inset-[-4px] rounded-xl border border-accent/35 animate-[spin_6s_linear_infinite_reverse]" />
          <Zap size={36} className="text-accent relative z-10" style={{ filter: "drop-shadow(0 0 12px var(--accent))" }} />
        </div>
        <div className="startup-splash__name">tianxuan</div>
        <div className="startup-splash__sub">{t("app.splashSubtitle") ?? "AI 编程助手"}</div>
        <div className="startup-splash__dots" aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
      </div>

      {/* capability cards */}
      <div className="grid grid-cols-2 gap-3 w-full max-w-sm mb-7">
        <FeatureCard icon={<Wrench size={15} />} color="var(--accent)" title={t("skeleton.tools")} desc={t("skeleton.toolsDesc")} />
        <FeatureCard icon={<Brain size={15} />} color="#a78bfa" title={t("skeleton.skills")} desc={t("skeleton.skillsDesc")} />
        <FeatureCard icon={<Blocks size={15} />} color="#38bdf8" title={t("skeleton.models")} desc={t("skeleton.modelsDesc")} />
        <FeatureCard icon={<Cpu size={15} />} color="#34d399" title={t("skeleton.cache")} desc={t("skeleton.cacheDesc")} />
      </div>

      <div className={`grid grid-cols-3 gap-2 mt-4 w-full ${compact ? "[&_button]:p-2 [&_button]:text-[11px]" : ""}`}>
        {QUICK_COMMANDS.map((cmd) => (
          <button
            key={cmd.label}
            className={`flex items-center gap-2 text-left font-[inherit] bg-bg-elev border border-border-soft text-fg-dim rounded-xl hover:text-fg hover:border-accent/20 hover:bg-bg-elev hover:-translate-y-px hover:shadow-[var(--ds-shadow-card)] transition-all ${compact ? "p-2 text-[11px]" : "p-2.5 text-[12px]"}`}
            onClick={() => onPrompt(cmd.prompt)}
            title={cmd.prompt}
          >
            <span className="text-fg-faint shrink-0">{cmd.icon}</span>
            <span className="font-medium truncate">{cmd.label}</span>
          </button>
        ))}
      </div>

      {recentSessions.length > 0 && onResumeSession && (
        <div className="w-full mt-5 pt-4 border-t border-border-soft">
          <div className={`font-semibold text-fg-faint uppercase tracking-wider mb-2.5 flex items-center gap-1.5 ${compact ? "text-[10px]" : "text-[11px]"}`}>
            <Clock size={12} />
            最近会话
          </div>
          <div className="flex flex-col gap-1.5">
            {recentSessions.map((s) => (
              <button
                key={s.path}
                className={`flex items-center gap-3 px-3 py-2.5 rounded-lg bg-bg-soft border border-border-soft text-left font-[inherit] text-fg-dim hover:text-fg hover:bg-bg-elev hover:border-fg-faint transition-all ${compact ? "text-[11px]" : "text-[12px]"}`}
                onClick={() => void onResumeSession(s.path)}
              >
                <MessageSquare size={compact ? 12 : 13} className="text-fg-faint shrink-0" />
                <span className="flex-1 truncate font-medium">{sessionTitle(s, "未命名会话")}</span>
                <span className={`text-fg-faint shrink-0 ${compact ? "text-[10px]" : "text-[11px]"}`}>{formatTimeAgo(s.modTime)}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function FeatureCard({ icon, title, desc, color }: { icon: React.ReactNode; title: string; desc: string; color: string }) {
  return (
    <div className="flex items-center gap-2.5 px-3 py-2.5 bg-bg-elev border border-border-soft rounded-lg transition-[border-color] duration-[var(--dur-fast)] hover:border-fg-faint/40">
      <span className="shrink-0" style={{ color }}>{icon}</span>
      <div className="flex flex-col min-w-0">
        <span className="text-[13px] font-medium text-fg truncate">{title}</span>
        <span className="text-[11px] text-fg-faint truncate">{desc}</span>
      </div>
    </div>
  );
}
