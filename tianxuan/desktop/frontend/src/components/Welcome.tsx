import { ArrowUp, FolderOpen, Bug, Code, Search, FileText, MessageSquare, Clock, Zap, PenTool, TestTube } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import logo from "../assets/logo.png";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import type { Meta, SessionMeta } from "../lib/types";

function sessionTitle(session: SessionMeta, fallback: string): string {
  return session.title || session.preview || fallback;
}

function formatTimeAgo(ms: number): string {
  const diff = Date.now() - ms;
  const min = Math.floor(diff / 60000);
  if (min < 1) return "刚刚";
  if (min < 60) return `${min}分钟前`;
  const hrs = Math.floor(min / 60);
  if (hrs < 24) return `${hrs}小时前`;
  return new Date(ms).toLocaleDateString([], { month: "short", day: "numeric" });
}

// 快捷命令 — 对标 Cline HomeView 的命令卡片
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
  const [text, setText] = useState("");
  const taRef = useRef<HTMLTextAreaElement>(null);
  const [showShortcuts, setShowShortcuts] = useState(false);

  // 快捷键横幅：首次显示，localStorage 记录
  useEffect(() => {
    try {
      if (!localStorage.getItem("tianxuan.shortcutsSeen")) {
        setShowShortcuts(true);
        localStorage.setItem("tianxuan.shortcutsSeen", "1");
        const timer = setTimeout(() => setShowShortcuts(false), 5000);
        return () => clearTimeout(timer);
      }
    } catch {}
  }, []);

  const handleSubmit = useCallback(() => {
    const trimmed = text.trim();
    if (!trimmed) return;
    onPrompt(trimmed);
    setText("");
  }, [text, onPrompt]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  const recentSessions = sessions?.filter(s => !s.current).slice(0, 3) ?? [];

  return (
    <div className="h-full flex flex-col items-center justify-center max-w-2xl mx-auto px-6 overflow-y-auto">
      {/* 项目语境提示 — 增强版：显示项目+分支+模型 */}
      {cwdName && (
        <div className={`inline-flex items-center gap-2 px-3 py-1.5 mb-5 rounded-full bg-accent-soft border border-accent/20 text-fg-dim ${compact ? "text-[11px]" : "text-[12px]"}`}>
          <FolderOpen size={compact ? 12 : 13} className="text-accent" />
          <span className="font-medium text-accent">{cwdName}</span>
          {meta?.label && <span className="text-fg-faint">· {meta.label}</span>}
        </div>
      )}

      {/* Logo + Tagline */}
      <img src={logo} className={`rounded-[10px] mb-3 ${compact ? "w-8 h-8" : "w-10 h-10"}`} alt="tianxuan" />
      <div className={`text-fg-dim mb-7 ${compact ? "text-[13px]" : "text-[14px]"}`} style={{fontFamily: "var(--ds-font-display)", fontWeight: 500, letterSpacing: "-0.01em"}}>{t("welcome.tagline")}</div>

      {/* 智能输入框 */}
      <div className="w-full border border-border-soft bg-bg-elev rounded-2xl shadow-[var(--ds-shadow-composer)] hover:border-fg-faint/30 focus-within:border-accent/30 focus-within:shadow-[0_0_0_1px_var(--accent-soft),var(--ds-shadow-composer)] transition-all duration-[var(--dur-base)]">
        <textarea
          ref={taRef}
          className={`w-full resize-none border-0 bg-transparent text-fg leading-relaxed outline-none placeholder:text-fg-faint px-5 pt-5 pb-2 ${compact ? "text-[13px] min-h-[64px]" : "text-[14px] min-h-[80px]"} max-h-[160px]`}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={cwdName ? `在 ${cwdName}/ 中提问…` : t("composer.placeholder")}
          rows={2}
        />
        <div className="flex items-center justify-between px-4 pb-4">
          <span className={`text-fg-faint ${compact ? "text-[10px]" : "text-[11px]"}`}>
            <kbd className="ds-kbd">/</kbd> 命令
            <span className="mx-1.5 text-fg-faint/40">·</span>
            <kbd className="ds-kbd">@</kbd> 文件
            <span className="mx-1.5 text-fg-faint/40">·</span>
            <kbd className="ds-kbd">↵</kbd> 发送
          </span>
          <button
            className={`inline-flex items-center justify-center w-8 h-8 border-0 rounded-full cursor-pointer shrink-0 transition-all duration-[var(--dur-fast)] active:scale-95 ${
              text.trim()
                ? "bg-accent text-accent-fg hover:brightness-110"
                : "bg-bg-elev-2 text-fg-faint"
            }`}
            style={text.trim() ? {boxShadow: "var(--ds-shadow-accent-btn)"} : undefined}
            onClick={handleSubmit}
            disabled={!text.trim()}
          >
            <ArrowUp size={16} />
          </button>
        </div>
      </div>

      {/* 快捷键横幅 — 首次引导 */}
      {showShortcuts && (
        <div className="w-full mt-3 animate-[toast-in_0.3s_ease-out]">
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-accent-soft border border-accent/15 text-[11px] text-fg-dim">
            <Code size={12} className="text-accent" />
            <span>
              <kbd className="font-mono text-accent bg-accent/10 rounded px-1 py-px text-[10px]">Enter</kbd> 发送
              <span className="mx-1.5 text-fg-faint">·</span>
              <kbd className="font-mono text-accent bg-accent/10 rounded px-1 py-px text-[10px]">Shift+Enter</kbd> 换行
              <span className="mx-1.5 text-fg-faint">·</span>
              <kbd className="font-mono text-accent bg-accent/10 rounded px-1 py-px text-[10px]">/</kbd> 命令
              <span className="mx-1.5 text-fg-faint">·</span>
              <kbd className="font-mono text-accent bg-accent/10 rounded px-1 py-px text-[10px]">@</kbd> 文件引用
            </span>
          </div>
        </div>
      )}

      {/* 快捷命令网格 — 对标 Cline HomeView 的命令面板 */}
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

      {/* 最近会话 — 增强为卡片式 */}
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
