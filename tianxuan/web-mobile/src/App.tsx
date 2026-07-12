// Mobile layout shell — 复用桌面端 zustand store，所有状态自动同步。
// 桌面端在 Wails 中，移动端在浏览器中，共享同一个 Controller + SSE 事件流。

import { useRef, useEffect, useCallback } from "react";
import { useController } from "@shared/lib/store";
import type { Item } from "@shared/lib/store";
import { UserMessage, AssistantMessage } from "@shared/components/Message";
import { AskCard } from "@shared/components/AskCard";

// ── 简易移动输入栏 ────────────────────────────────────────────────────
function MobileComposer({ running, onSend, onCancel }: { running: boolean; onSend: (text: string) => void; onCancel: () => void }) {
  const ref = useRef<HTMLTextAreaElement>(null);

  const submit = useCallback(() => {
    const text = ref.current?.value.trim();
    if (text && !running) { onSend(text); ref.current!.value = ""; ref.current!.style.height = "auto"; }
  }, [running, onSend]);

  const resize = () => {
    const el = ref.current; if (!el) return;
    el.style.height = "auto"; el.style.height = Math.min(el.scrollHeight, 100) + "px";
  };

  return (
    <div className="safe-bottom border-t border-border bg-bg/90 backdrop-blur-xl px-2.5 py-2.5">
      <form onSubmit={(e) => { e.preventDefault(); submit(); }} className="flex items-end gap-2">
        <div className="flex-1 bg-white/5 rounded-2xl border border-white/10 focus-within:border-accent/30 transition-colors overflow-hidden">
          <textarea ref={ref} onChange={resize}
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submit(); } }}
            placeholder="输入消息…" rows={1} disabled={running}
            className="w-full bg-transparent text-fg placeholder:text-fg-muted text-[16px] px-3.5 py-2.5 outline-none resize-none disabled:opacity-40"
            style={{ minHeight: 44, maxHeight: 100, fontSize: 16 }} />
        </div>
        {running ? (
          <button type="button" onClick={onCancel}
            className="w-11 h-11 rounded-2xl bg-danger/15 border border-danger/20 flex items-center justify-center active:scale-95 shrink-0">
            <div className="w-3 h-3 rounded-sm bg-danger" />
          </button>
        ) : (
          <button type="submit"
            className="w-11 h-11 rounded-2xl bg-accent flex items-center justify-center active:scale-95 shrink-0 shadow-lg shadow-accent/25">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5" strokeLinecap="round"><line x1="12" y1="19" x2="12" y2="5" /><polyline points="5 12 12 5 19 12" /></svg>
          </button>
        )}
      </form>
    </div>
  );
}

// ── 消息列表 ──────────────────────────────────────────────────────────
function MessageList({ items, running }: { items: Item[]; running: boolean }) {
  const bottomRef = useRef<HTMLDivElement>(null);
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [items.length]);

  return (
    <div className="flex-1 overflow-y-auto px-3 py-3 space-y-3">
      {items.length === 0 && !running && (
        <div className="flex flex-col items-center justify-center h-full text-fg-muted pt-16">
          <p className="text-sm">发送消息开始对话</p>
        </div>
      )}
      {items.map((item: any) => {
        if (item.kind === "user") return <UserMessage key={item.id} text={item.text} />;
        if (item.kind === "assistant" || item.kind === "assistant_header") return <AssistantMessage key={item.id} item={item} />;
        if (item.kind === "notice") return <div key={item.id} className="text-xs text-fg-dim text-center py-1">{item.text}</div>;
        if (item.kind === "tools" || item.kind === "tool_call" || item.kind === "tool_result") return null; // 工具信息已在 assistant 消息中显示
        return null;
      })}
      {running && items.length > 0 && items[items.length - 1]?.kind === "user" && (
        <div className="flex items-center gap-2 px-1 py-2">
          <div className="flex gap-1">{ [0, 200, 400].map((d) => (<div key={d} className="w-1.5 h-1.5 rounded-full bg-fg-muted animate-bounce" style={{ animationDelay: `${d}ms`, animationDuration: "0.6s" }} />))}</div>
          <span className="text-xs text-fg-muted">思考中…</span>
        </div>
      )}
      <div ref={bottomRef} />
    </div>
  );
}

// ── 顶部栏 ────────────────────────────────────────────────────────────
function MobileHeader() {
  const ctrl = useController();
  const s = ctrl.state;

  return (
    <div className="safe-top glass border-b border-border px-3 py-2">
      <div className="flex items-center gap-2">
        <span className="text-xs text-fg-dim truncate flex-1">
          {s.context?.window ? `${Math.round((s.context.used / s.context.window) * 100)}% · ` : ""}
          {s.meta?.label || ""}
        </span>
        <div className={`w-2 h-2 rounded-full ${s.running ? "bg-accent shadow-[0_0_6px] shadow-accent/60 animate-pulse" : "bg-success"}`} />
      </div>
    </div>
  );
}

// ── App ───────────────────────────────────────────────────────────────
export default function App() {
  const ctrl = useController();
  const s = ctrl.state;

  return (
    <div className="flex flex-col h-full bg-bg-deep">
      <MobileHeader />
      <MessageList items={s.items} running={s.running} />
      <MobileComposer running={s.running} onSend={ctrl.send} onCancel={ctrl.cancel} />
      {s.ask && <AskCard ask={s.ask} onAnswer={(id, answers) => ctrl.answerQuestion(id, answers)} onDismiss={() => {}} />}
    </div>
  );
}
