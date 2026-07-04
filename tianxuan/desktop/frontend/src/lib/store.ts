// Zustand store — replaces useController's useReducer state machine.

import { useCallback, useEffect } from "react";
import { create } from "zustand";
import { app, onEvent, onReady } from "./bridge";
import type {
  BalanceInfo, ContextInfo, HistoryMessage, JobView, MemoryView,
  Meta, QuestionAnswer, SessionMeta, TCCAReport, WireApproval, WireAsk,
  WireEvent, WireUsage,
} from "./types";

export type ToolStatus = "running" | "done" | "error" | "stopped";

export type Item =
  | { kind: "user"; id: string; text: string }
  | { kind: "assistant"; id: string; text: string; reasoning: string; streaming: boolean }
  | { kind: "phase"; id: string; text: string }
  | { kind: "notice"; id: string; level: "info" | "warn"; text: string }
  | { kind: "compaction"; id: string; pending: boolean; trigger: string; messages: number; summary: string; archive: string }
  | { kind: "tool"; id: string; name: string; args: string; readOnly: boolean; status: ToolStatus; output?: string; error?: string; truncated?: boolean; recoverable?: boolean; parentId?: string };

interface ControllerState {
  items: Item[]; running: boolean; turnActive: boolean; approval?: WireApproval; ask?: WireAsk;
  usage?: WireUsage; context: ContextInfo; meta?: Meta; balance?: BalanceInfo; jobs: JobView[];
  tcca?: TCCAReport;
  currentAssistant?: string; pendingUser?: string; discardTurn?: boolean;
  lastAssistantIdx: number; // 最后一个 assistant 项的索引，避免流式 text/reasoning 事件中 O(n) 反向查找
  turnStartAt: number; turnTokens: number; seq: number;
  sessionTotal: number;
  perTurnUsage: WireUsage | null | undefined; // V5.30: whole-turn accumulated usage
  perTurnMainUsage?: WireUsage; // V10.22: main agent usage only
  perTurnSubUsage?: WireUsage;  // V10.22: subagent usage only
  turnSteps: WireUsage[]; // V5.31: raw per-step usage within current turn
  sessionNonce: number; // V5.25: 每次新建/恢复会话递增，确保统计面板按会话区分
  _dispatch: (a: Action) => void;
}

type Action =
  | { type: "event"; e: WireEvent } | { type: "user"; text: string } | { type: "unsend" }
  | { type: "meta"; meta: Meta } | { type: "context"; context: ContextInfo }
  | { type: "balance"; balance: BalanceInfo } | { type: "jobs"; jobs: JobView[] }
  | { type: "tcca"; report: TCCAReport }
  | { type: "history"; messages: HistoryMessage[] } | { type: "clearApproval" } | { type: "clearAsk" } | { type: "reset" };


function flushPendingUser(s: ControllerState): ControllerState {
  if (s.pendingUser === undefined) return s;
  // 如果消息已通过 user action 立即加入 items，只清除 pendingUser 避免重复
  const last = s.items.length > 0 ? s.items[s.items.length - 1] : null;
  if (last && last.kind === "user" && last.text === s.pendingUser) {
    return { ...s, pendingUser: undefined };
  }
  return { ...s, seq: s.seq + 1, items: [...s.items, { kind: "user", id: `u${s.seq}`, text: s.pendingUser }], pendingUser: undefined };
}


function applyEvent(s: ControllerState, e: WireEvent): ControllerState {
  if (s.discardTurn) { if (e.kind === "turn_done") return { ...s, discardTurn: false, running: false, turnActive: false, currentAssistant: undefined }; return s; }
  if (s.pendingUser !== undefined && e.kind !== "turn_started" && e.kind !== "turn_done") s = flushPendingUser(s);
  switch (e.kind) {
    case "turn_started": return { ...s, running: true, turnActive: true, currentAssistant: undefined, lastAssistantIdx: -1, turnStartAt: Date.now(), turnTokens: 0, perTurnUsage: null, perTurnMainUsage: undefined, perTurnSubUsage: undefined, turnSteps: [] };
    case "text": case "reasoning": {
      // O(1) 查找最后一个 assistant 项：用 lastAssistantIdx 避免流式时每 chunk O(n) 扫描。
      // 若最后 assistant 已终结（上一轮 turn_done 已将 streaming 置 false）且当前轮活跃，
      // 则创建新项而非追加到旧轮次消息——修复跨轮次文本覆盖。
      const delta = e.text ?? e.reasoning ?? "";
      let idx = s.lastAssistantIdx;
      // 验证缓存索引有效性（非流式事件间可能有 items 变更）
      if (idx < 0 || idx >= s.items.length || s.items[idx].kind !== "assistant") {
        for (let i = s.items.length - 1; i >= 0; i--) {
          if (s.items[i].kind === "assistant") { idx = i; break; }
        }
      }
      const needNew = idx < 0 || (
        (s.items[idx] as Extract<Item, { kind: "assistant" }>).streaming === false &&
        s.turnActive
      );
      if (!needNew) {
        const it = s.items[idx] as Extract<Item, { kind: "assistant" }>;
        const next = [...s.items];
        next[idx] = e.kind === "text"
          ? { ...it, text: it.text + delta, streaming: true }
          : { ...it, reasoning: it.reasoning + delta, streaming: true };
        return { ...s, items: next, currentAssistant: it.id, lastAssistantIdx: idx };
      }
      // 没有可追加的活跃 assistant 项时创建新的
      const id = `a${s.seq}`;
      const newIdx = s.items.length;
      return { ...s, seq: s.seq + 1, items: [...s.items, { kind: "assistant", id, text: e.kind === "text" ? delta : "", reasoning: e.kind === "reasoning" ? delta : "", streaming: true }], currentAssistant: id, lastAssistantIdx: newIdx };
    }
    case "message": {
      // 始终更新最后一个 assistant，不创建新的。
      // 若最后 assistant 已终结（上一轮结束）且当前轮活跃，则创建新项
      // 而非覆盖旧轮次消息——修复跨轮次文本覆盖。
      let idx = s.lastAssistantIdx;
      if (idx < 0 || idx >= s.items.length || s.items[idx].kind !== "assistant") {
        for (let i = s.items.length - 1; i >= 0; i--) {
          if (s.items[i].kind === "assistant") { idx = i; break; }
        }
      }
      const needNew = idx < 0 || (
        (s.items[idx] as Extract<Item, { kind: "assistant" }>).streaming === false &&
        s.turnActive
      );
      if (!needNew) {
        const it = s.items[idx] as Extract<Item, { kind: "assistant" }>;
        const next = [...s.items];
        next[idx] = { ...it, text: e.text ?? it.text, reasoning: e.reasoning ?? it.reasoning, streaming: false };
        return { ...s, items: next, currentAssistant: undefined, lastAssistantIdx: idx };
      }
      // 没有任何可更新的 assistant 项时创建新的（首轮且模型直接回了 message）
      const id = `a${s.seq}`;
      const newIdx = s.items.length;
      return { ...s, seq: s.seq + 1, items: [...s.items, { kind: "assistant", id, text: e.text ?? "", reasoning: e.reasoning ?? "", streaming: false }], currentAssistant: undefined, lastAssistantIdx: newIdx };
    }
    case "tool_dispatch": {
      const t = e.tool; if (!t) return s;
      const id = t.id || `tool${s.seq}`;
      const idx = s.items.findIndex(it => it.kind === "tool" && it.id === id);
      if (idx >= 0) { const next = [...s.items]; const it = next[idx]; if (it.kind === "tool") next[idx] = { ...it, name: t.name, args: t.args ? t.args : it.args, readOnly: t.readOnly }; return { ...s, currentAssistant: undefined, items: next }; }
      return { ...s, seq: s.seq + 1, currentAssistant: undefined, items: [...s.items, { kind: "tool", id, name: t.name, args: t.args ?? "", readOnly: t.readOnly, status: "running", parentId: t.parentId }] };
    }
    case "tool_result": {
      const t = e.tool; if (!t) return s; const next = [...s.items];
      let idx = t.id ? next.findIndex(it => it.kind === "tool" && it.id === t.id) : -1;
      // Fallback: no exact ID match — find the last still-running tool
      if (idx < 0) {
        for (let i = next.length - 1; i >= 0; i--) {
          const cand = next[i];
          if (cand.kind === "tool" && (cand as any).status === "running") { idx = i; break; }
        }
      }
      if (idx >= 0) { const it = next[idx]; if (it.kind === "tool") next[idx] = { ...it, status: t.err ? "error" : "done", output: t.output, error: t.err, recoverable: t.recoverable, truncated: t.truncated }; }
      return { ...s, items: next };
    }
    case "usage": {
      const used = e.usage && s.context.window ? e.usage.promptTokens : s.context.used;
      const u = e.usage;
      // combined accumulator (backwards compat)
      const acc = s.perTurnUsage && u ? {
        promptTokens: s.perTurnUsage.promptTokens + (u.promptTokens ?? 0),
        completionTokens: s.perTurnUsage.completionTokens + (u.completionTokens ?? 0),
        totalTokens: s.perTurnUsage.totalTokens + (u.totalTokens ?? 0),
        cacheHitTokens: s.perTurnUsage.cacheHitTokens + (u.cacheHitTokens ?? 0),
        cacheMissTokens: s.perTurnUsage.cacheMissTokens + (u.cacheMissTokens ?? 0),
        sessionCacheHitTokens: u.sessionCacheHitTokens > 0 ? u.sessionCacheHitTokens : (s.perTurnUsage?.sessionCacheHitTokens ?? 0),
        sessionCacheMissTokens: u.sessionCacheMissTokens > 0 ? u.sessionCacheMissTokens : (s.perTurnUsage?.sessionCacheMissTokens ?? 0),
      } : u;
      // V10.22: split by source — main vs subagent
      const isSub = u?.source === "subagent";
      const prevMain = s.perTurnMainUsage, prevSub = s.perTurnSubUsage;
      const accBySource = (prev?: WireUsage) => prev && u ? {
        promptTokens: prev.promptTokens + (u.promptTokens ?? 0),
        completionTokens: prev.completionTokens + (u.completionTokens ?? 0),
        totalTokens: prev.totalTokens + (u.totalTokens ?? 0),
        cacheHitTokens: prev.cacheHitTokens + (u.cacheHitTokens ?? 0),
        cacheMissTokens: prev.cacheMissTokens + (u.cacheMissTokens ?? 0),
        sessionCacheHitTokens: u.sessionCacheHitTokens > 0 ? u.sessionCacheHitTokens : (prev.sessionCacheHitTokens ?? 0),
        sessionCacheMissTokens: u.sessionCacheMissTokens > 0 ? u.sessionCacheMissTokens : (prev.sessionCacheMissTokens ?? 0),
      } : u;
      const tagged = u ? { ...u } : undefined; const steps = tagged ? [...s.turnSteps, tagged] : s.turnSteps;
      return { ...s, usage: tagged, perTurnUsage: acc, perTurnMainUsage: accBySource(isSub ? prevMain : u ? (prevMain ?? (u as WireUsage)) : prevMain), perTurnSubUsage: accBySource(isSub ? (u ? (prevSub ?? (u as WireUsage)) : prevSub) : prevSub), turnSteps: steps, context: { ...s.context, used }, turnTokens: s.turnTokens + (tagged?.completionTokens ?? 0) };
    }
    case "notice": return { ...s, running: s.turnActive ? s.running : false, seq: s.seq + 1, items: [...s.items, { kind: "notice", id: `n${s.seq}`, level: e.level ?? "info", text: e.text ?? "" }] };
    case "phase": return { ...s, seq: s.seq + 1, items: [...s.items, { kind: "phase", id: `p${s.seq}`, text: e.text ?? "" }] };
    case "approval_request": return { ...s, approval: e.approval };
    case "ask_request": return { ...s, ask: e.ask };
    case "turn_done": {
      if (s.pendingUser !== undefined) s = flushPendingUser(s);
      const finalized = s.items.map(it => { if (it.kind === "assistant" && it.streaming) return { ...it, streaming: false }; if (it.kind === "tool" && it.status === "running") return { ...it, status: "stopped" as const }; return it; });
      const finalItems: Item[] = e.err ? [...finalized, { kind: "notice", id: `e${s.seq}`, level: "warn", text: e.err }] : finalized;
      const st = (s.usage?.totalTokens != null && s.usage.totalTokens > 0) ? s.sessionTotal + s.usage.totalTokens : s.sessionTotal;
      // V5.30: 设 perTurnUsage=null 触发 StatsPanel 创建末轮 TurnRecord
      return { ...s, items: finalItems, running: false, turnActive: false, currentAssistant: undefined, lastAssistantIdx: -1, approval: undefined, ask: undefined, perTurnUsage: null, seq: s.seq + 1, sessionTotal: st };
    }
    default: return s;
  }
  return s;
}

function reducer(s: ControllerState, a: Action): ControllerState {
  switch (a.type) {
    case "user": return { ...s, running: true, turnStartAt: Date.now(), turnTokens: 0, pendingUser: a.text, discardTurn: false, seq: s.seq + 1, items: [...s.items, { kind: "user", id: `u${s.seq}`, text: a.text }] };
    case "unsend": return { ...s, pendingUser: undefined, discardTurn: true, running: false };
    case "meta": return { ...s, meta: a.meta }; case "context": return { ...s, context: a.context };
    case "balance": return { ...s, balance: a.balance }; case "jobs": return { ...s, jobs: a.jobs };
    case "tcca": return { ...s, tcca: a.report };
    case "history": { const visible = a.messages.filter(m => (m.role === "user" || m.role === "assistant") && m.content.trim() !== ""); const lastIdx = visible.reduceRight((acc, m, i) => acc >= 0 ? acc : m.role === "assistant" ? i : -1, -1); return { ...s, items: visible.map((m, i) => m.role === "user" ? { kind: "user", id: `h${i}`, text: m.content } as Item : { kind: "assistant", id: `h${i}`, text: m.content, reasoning: "", streaming: false } as Item), seq: s.seq + visible.length, lastAssistantIdx: lastIdx }; }
    case "clearApproval": return { ...s, approval: undefined }; case "clearAsk": return { ...s, ask: undefined };
    case "reset": return { ...initialState, meta: s.meta, context: { ...s.context, used: 0 }, balance: s.balance, jobs: s.jobs, seq: s.seq, sessionNonce: s.sessionNonce + 1, _dispatch: s._dispatch };
    case "event": return applyEvent(s, a.e);
    default: return s;
  }
  return s;
}

const initialState: ControllerState = {
  items: [], running: false, turnActive: false,
  approval: undefined, ask: undefined, usage: undefined,
  context: { used: 0, window: 0 }, meta: undefined, balance: undefined,
  tcca: undefined,
  jobs: [], currentAssistant: undefined, pendingUser: undefined, discardTurn: false, lastAssistantIdx: -1,
  turnStartAt: 0, turnTokens: 0, seq: 0, sessionTotal: 0, sessionNonce: 0, perTurnUsage: null, turnSteps: [],
  _dispatch: () => {},
};

export const useStore = create<ControllerState>()((set) => ({ ...initialState, _dispatch: (a: Action) => set((s) => reducer(s, a)) } as ControllerState));

export function useController() {
  const store = useStore;
  const state = store(s => s);
  const dispatch = store.getState()._dispatch;

  const loadSessionData = useCallback(async () => {
    try {
      dispatch({ type: "meta", meta: await app.Meta() });
      dispatch({ type: "context", context: await app.ContextUsage() });
      const history = await app.History();
      if (history && history.length) dispatch({ type: "history", messages: history });
    } catch {}
  }, [dispatch]);

  useEffect(() => {
    const off = onEvent((e) => {
      // 流式 text/reasoning 用 queueMicrotask 确保每次 chunk 即时渲染，
      // 不被 React 18 自动批处理合并。同步 dispatch 会导致多个事件在同一
      // 微任务中批量更新从而不渲染中间态。
      if (e.kind === "text" || e.kind === "reasoning") {
        queueMicrotask(() => dispatch({ type: "event", e }));
      } else {
        dispatch({ type: "event", e });
      }
      if (e.kind === "turn_done") {
        app.ContextUsage().then(c => dispatch({ type: "context", context: c })).catch(() => {});
        app.Balance().then(b => dispatch({ type: "balance", balance: b })).catch(() => {});
        app.TCCAReport().then(raw => {
          try { dispatch({ type: "tcca", report: JSON.parse(raw) as TCCAReport }); } catch {}
        }).catch(() => {});
      }
      if (e.kind === "turn_done" || e.kind === "notice") {
        app.Jobs().then(j => dispatch({ type: "jobs", jobs: j })).catch(() => {});
      }
    });
    const offReady = onReady(() => {
      void loadSessionData();
      app.Balance().then(b => dispatch({ type: "balance", balance: b })).catch(() => {});
      app.Jobs().then(j => dispatch({ type: "jobs", jobs: j })).catch(() => {});
      app.TCCAReport().then(raw => {
        try { dispatch({ type: "tcca", report: JSON.parse(raw) as TCCAReport }); } catch {}
      }).catch(() => {});
    });
    void loadSessionData();
    app.Balance().then(b => dispatch({ type: "balance", balance: b })).catch(() => {});
    app.Jobs().then(j => dispatch({ type: "jobs", jobs: j })).catch(() => {});
    return () => { off(); offReady(); };
  }, [loadSessionData]);

  const send = useCallback((displayText: string, submitText = displayText) => {
    dispatch({ type: "user", text: displayText });
    const display = displayText.trim(); const submit = submitText.trim();
    (display !== submit ? app.SubmitDisplay(display, submit) : app.Submit(submit)).catch(() => {});
  }, [dispatch]);

  const cancel = useCallback((): string | undefined => {
    const cur = store.getState();
    if (cur.running && cur.pendingUser !== undefined) { const text = cur.pendingUser; dispatch({ type: "unsend" }); app.Cancel().catch(() => {}); return text; }
    app.Cancel().catch(() => {}); return undefined;
  }, [store, dispatch]);

  const approve = useCallback((id: string, allow: boolean, session: boolean) => { dispatch({ type: "clearApproval" }); app.Approve(id, allow, session).catch(() => {}); }, [dispatch]);
  const answerQuestion = useCallback((id: string, answers: QuestionAnswer[]) => { dispatch({ type: "clearAsk" }); app.AnswerQuestion(id, answers).catch(() => {}); }, [dispatch]);
  const setPermLevel = useCallback((level: string) => { app.SetPermLevel(level).catch(() => {}); }, []);
  const newSession = useCallback(async () => { await app.NewSession().catch(() => {}); dispatch({ type: "reset" }); }, [dispatch]);
  const listSessions = useCallback((): Promise<SessionMeta[]> => app.ListSessions().catch(() => []), []);
  const resumeSession = useCallback(async (path: string) => { const ms = await app.ResumeSession(path).catch(() => [] as HistoryMessage[]); dispatch({ type: "reset" }); if (ms.length) dispatch({ type: "history", messages: ms }); app.ContextUsage().then(c => dispatch({ type: "context", context: c })).catch(() => {}); }, [dispatch]);
  const deleteSession = useCallback((path: string) => app.DeleteSession(path).catch(() => {}), []);
  const renameSession = useCallback((path: string, title: string) => app.RenameSession(path, title).catch(() => {}), []);
  const refreshMeta = useCallback(async () => { try { dispatch({ type: "meta", meta: await app.Meta() }); dispatch({ type: "context", context: await app.ContextUsage() }); } catch {} }, [dispatch]);
  const pickWorkspace = useCallback(async (): Promise<string> => { const p = await app.PickWorkspace().catch(() => ""); if (p) { dispatch({ type: "reset" }); try { dispatch({ type: "meta", meta: await app.Meta() }); dispatch({ type: "context", context: await app.ContextUsage() }); } catch {} } return p; }, [dispatch]);
  const switchWorkspace = useCallback(async (path: string): Promise<string> => { const n = await app.SwitchWorkspace(path).catch(() => ""); if (n) { dispatch({ type: "reset" }); try { dispatch({ type: "meta", meta: await app.Meta() }); dispatch({ type: "context", context: await app.ContextUsage() }); } catch {} } return n; }, [dispatch]);
  const compact = useCallback(() => { app.Compact().catch(() => {}); }, []);
  const setModel = useCallback(async (name: string) => { await app.SetModel(name).catch(() => {}); try { dispatch({ type: "meta", meta: await app.Meta() }); dispatch({ type: "context", context: await app.ContextUsage() }); } catch {} }, [dispatch]);
  const fetchMemory = useCallback((): Promise<MemoryView> => app.Memory().catch(() => ({ docs: [], facts: [], scopes: [], storeDir: "", available: false } as MemoryView)), []);
  const remember = useCallback(async (scope: string, note: string) => { await app.Remember(scope, note).catch(() => {}); }, []);
  const forget = useCallback(async (name: string) => { await app.Forget(name).catch(() => {}); }, []);
  const saveDoc = useCallback(async (path: string, body: string) => { await app.SaveDoc(path, body).catch(() => {}); }, []);
  const updateFact = useCallback(async (name: string, body: string) => { await app.UpdateFact(name, body).catch(() => {}); }, []);
  const changeFactType = useCallback(async (name: string, typ: string) => { await app.ChangeFactType(name, typ).catch(() => {}); }, []);
  const rewind = useCallback(async (turn: number, scope: string) => { if (scope === "fork") await app.Fork(turn).catch(() => {}); else if (scope === "summ-from") await app.SummarizeFrom(turn).catch(() => {}); else if (scope === "summ-upto") await app.SummarizeUpTo(turn).catch(() => {}); else await app.Rewind(turn, scope).catch(() => {}); const ms = await app.History().catch(() => [] as HistoryMessage[]); dispatch({ type: "reset" }); if (ms.length) dispatch({ type: "history", messages: ms }); app.ContextUsage().then(c => dispatch({ type: "context", context: c })).catch(() => {}); }, [dispatch]);

  return { state, send, cancel, approve, answerQuestion,  setPermLevel, newSession, listSessions, resumeSession, deleteSession, renameSession, refreshMeta, pickWorkspace, switchWorkspace, compact, rewind, setModel, fetchMemory, remember, forget, saveDoc, updateFact, changeFactType };
}

// useItems 订阅 items 数组，与 useController 分离。
// 流式输出时 items 高频变化（每次 text/reasoning 事件），通过独立 hook 避免
// useController 的 store(s=>s) 全量订阅导致 App 树全局重渲染。
// 使用 useShallow 做浅比较：仅当 items 长度或元素引用变化时才触发重渲染，
// 非 items 字段（meta/context/balance 等）的变化不会影响此 hook。
// useItems 订阅 items 数组，与 useController 分离。
// 流式输出时 items 高频变化（每次 text/reasoning 事件），通过独立 hook 避免
// useController 的 store(s=>s) 全量订阅导致 App 树全局重渲染。
export function useItems(): Item[] {
  return useStore(s => s.items);
}

// useTurnStartAt 返回当前回合开始时间戳(ms)，用于计算思考耗时。
export function useTurnStartAt(): number {
  return useStore(s => s.turnStartAt);
}
