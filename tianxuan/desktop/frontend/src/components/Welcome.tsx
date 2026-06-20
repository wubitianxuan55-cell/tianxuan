import { ArrowUp, FolderOpen, Bug, Lightbulb, Code, Search, FileText } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import logo from "../assets/logo.png";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import type { Meta, SessionMeta } from "../lib/types";

interface StarterCard {
  icon: React.ReactNode;
  title: string;
  desc: string;
  prompt: string;
}

function sessionTime(ms: number): string {
  return new Date(ms).toLocaleDateString([], { month: "short", day: "numeric" });
}

function sessionTitle(session: SessionMeta, fallback: string): string {
  return session.title || session.preview || fallback;
}

export function Welcome({
  onPrompt,
  cwd,
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

  // 快捷键横幅：首次显示，localStorage 记录（Wails webview 更可靠）
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

  // 动态起始卡片
  const cards: StarterCard[] = (() => {
    const base: StarterCard[] = [];
    base.push({
      icon: <Search size={compact ? 16 : 18} />,
      title: t("welcome.card1.title") || "探索代码库",
      desc: cwdName ? `了解 ${cwdName} 项目结构和关键模块` : (t("welcome.card1.desc") || "了解项目结构和关键模块"),
      prompt: cwd
        ? "explore this codebase — identify the key modules, their responsibilities, and how they connect"
        : (t("welcome.card1.prompt") || "explore this codebase"),
    });
    base.push({
      icon: <Bug size={compact ? 16 : 18} />,
      title: t("welcome.card2.title") || "修复 Bug",
      desc: t("welcome.card2.desc") || "描述你遇到的问题，我来定位和修复",
      prompt: t("welcome.card2.prompt") || "there is a bug — when I ... it does ... instead of ...",
    });
    base.push({
      icon: <Lightbulb size={compact ? 16 : 18} />,
      title: t("welcome.card3.title") || "添加功能",
      desc: t("welcome.card3.desc") || "描述你想添加的新功能",
      prompt: t("welcome.card3.prompt") || "add a feature: ",
    });
    return base;
  })();

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
      {/* 项目语境提示 */}
      {cwdName && (
        <div className={`inline-flex items-center gap-2 px-3 py-1.5 mb-5 rounded-full bg-accent-soft border border-accent/20 text-fg-dim ${compact ? "text-[11px]" : "text-[12px]"}`}>
          <FolderOpen size={compact ? 12 : 13} className="text-accent" />
          <span className="font-medium text-accent">{cwdName}</span>
          {meta?.label && <span className="text-fg-faint">· {meta.label}</span>}
        </div>
      )}

      {/* Logo */}
      <img src={logo} className={`rounded-[10px] mb-3 ${compact ? "w-8 h-8" : "w-10 h-10"}`} alt="tianxuan" />
      <div className={`text-fg-dim mb-7 ${compact ? "text-[12.5px]" : "text-[13.5px]"}`}>{t("welcome.tagline")}</div>

      {/* Central input box */}
      <div className="w-full border border-border rounded-xl bg-bg-soft shadow-sm hover:border-fg-faint focus-within:border-accent transition-colors duration-[0.15s]">
        <textarea
          ref={taRef}
          className={`w-full resize-none border-0 bg-transparent text-fg leading-relaxed outline-none placeholder:text-fg-faint px-4 pt-4 pb-2 ${compact ? "text-[13px]" : "text-[14px]"}`}
          style={{ minHeight: compact ? "64px" : "80px", maxHeight: "160px" }}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={cwdName ? `在 ${cwdName}/ 中提问…` : t("composer.placeholder")}
          rows={2}
        />
        <div className="flex items-center justify-between px-3 pb-3">
          <span className={`text-fg-faint ${compact ? "text-[10px]" : "text-[11px]"}`}>
            <kbd className="font-mono text-fg-dim bg-bg-elev-2 border border-border rounded px-1 py-px text-[10px]">/</kbd> 命令
            <span className="mx-1.5">·</span>
            <kbd className="font-mono text-fg-dim bg-bg-elev-2 border border-border rounded px-1 py-px text-[10px]">@</kbd> 文件
            <span className="mx-1.5">·</span>
            <kbd className="font-mono text-fg-dim bg-bg-elev-2 border border-border rounded px-1 py-px text-[10px]">↵</kbd> 发送
          </span>
          <button
            className={`inline-flex items-center justify-center w-7 h-7 border-0 rounded-md cursor-pointer shrink-0 transition-all duration-[0.12s] active:scale-95 ${
              text.trim()
                ? "bg-accent text-accent-fg hover:brightness-110"
                : "bg-bg-elev-2 text-fg-faint"
            }`}
            onClick={handleSubmit}
            disabled={!text.trim()}
          >
            <ArrowUp size={15} />
          </button>
        </div>
      </div>

      {/* 快捷键横幅 */}
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

      {/* Starter cards grid */}
      <div className={`grid grid-cols-3 gap-2.5 mt-6 w-full ${compact ? "[&_button]:p-2 [&_button]:text-[12px]" : ""}`}>
        {cards.map((card) => (
          <button
            key={card.title}
            className={`flex flex-col items-start gap-2 text-left font-[inherit] bg-bg-soft border border-border-soft text-fg-dim rounded-lg hover:text-fg hover:border-accent-soft hover:bg-bg-elev hover:-translate-y-px transition-all ${compact ? "p-2 text-[12px]" : "p-3 text-[13px]"}`}
            onClick={() => onPrompt(card.prompt)}
          >
            <span className="text-fg-dim">{card.icon}</span>
            <span className="flex flex-col gap-0.5">
              <span className={`font-medium text-fg ${compact ? "text-[12px]" : "text-[13px]"}`}>{card.title}</span>
              <span className={`text-fg-faint leading-snug ${compact ? "text-[11px]" : "text-[12px]"}`}>{card.desc}</span>
            </span>
          </button>
        ))}
      </div>

      {/* 最近会话迷你列表 */}
      {recentSessions.length > 0 && onResumeSession && (
        <div className="w-full mt-7 pt-5 border-t border-border-soft">
          <div className={`font-semibold text-fg-faint uppercase tracking-wider mb-3 flex items-center gap-1.5 ${compact ? "text-[10px]" : "text-[11px]"}`}>
            <FileText size={12} />
            最近会话
          </div>
          <div className="flex flex-col gap-1">
            {recentSessions.map((s) => (
              <button
                key={s.path}
                className={`flex items-center gap-2.5 px-3 py-2 rounded-lg bg-bg-soft border border-border-soft text-left font-[inherit] text-fg-dim hover:text-fg hover:bg-bg-elev hover:border-fg-faint transition-colors ${compact ? "text-[11.5px]" : "text-[12.5px]"}`}
                onClick={() => void onResumeSession(s.path)}
              >
                <span className="flex-1 truncate font-medium">{sessionTitle(s, "未命名会话")}</span>
                <span className={`text-fg-faint shrink-0 ${compact ? "text-[10px]" : "text-[11px]"}`}>{sessionTime(s.modTime)}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
