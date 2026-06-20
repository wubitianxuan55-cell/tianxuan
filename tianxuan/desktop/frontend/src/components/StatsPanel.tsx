import { useEffect, useRef, useState } from "react";
import { BarChart3, Zap, Hash, Gauge, Footprints } from "lucide-react";
import type { WireUsage, ContextInfo } from "../lib/types";

function storageKey(sessionKey: string) { return `tianxuan.stats.${sessionKey}`; }

interface StoredData { turns: TurnRecord[]; steps: StepRecord[]; }

function loadData(sessionKey: string): StoredData {
  try {
    const raw = localStorage.getItem(storageKey(sessionKey));
    if (!raw) return { turns: [], steps: [] };
    const parsed = JSON.parse(raw);
    // 兼容旧格式：V5.30 之前存储的是 TurnRecord[] 数组
    if (Array.isArray(parsed)) return { turns: parsed as TurnRecord[], steps: [] };
    // 新格式：StoredData { turns, steps }
    return {
      turns: Array.isArray((parsed as StoredData).turns) ? (parsed as StoredData).turns : [],
      steps: Array.isArray((parsed as StoredData).steps) ? (parsed as StoredData).steps : [],
    };
  } catch { return { turns: [], steps: [] }; }
}

function saveData(sessionKey: string, data: StoredData) {
  try { localStorage.setItem(storageKey(sessionKey), JSON.stringify(data)); } catch {}
}

interface TurnRecord {
  turn: number;
  prompt: number;
  completion: number;
  cacheHit: number;
  cacheMiss: number;
  cost: number;
  totalTokens: number;
}

interface StepRecord {
  step: number;
  prompt: number;
  completion: number;
  cacheHit: number;
  cacheMiss: number;
}

function fmt(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  return String(n);
}

function pct(n: number, d: number): string {
  if (d <= 0) return "—";
  return (n / d * 100).toFixed(2) + "%";
}

const MODEL_PRICES: Record<string, { cacheHit: number; input: number; output: number; label: string }> = {
  "deepseek-v4-flash": { cacheHit: 0.0203, input: 1.015, output: 2.03, label: "V4 Flash" },
  "deepseek-v4-pro":   { cacheHit: 0.0263, input: 3.154, output: 6.308, label: "V4 Pro" },
};
const DEFAULT_PRICE = MODEL_PRICES["deepseek-v4-flash"];

function modelPrice(label?: string) {
  if (!label) return DEFAULT_PRICE;
  for (const [key, p] of Object.entries(MODEL_PRICES)) {
    if (label.includes(key)) return p;
  }
  return DEFAULT_PRICE;
}

function calcCost(tokens: number, pricePerM: number): number {
  return (tokens / 1_000_000) * pricePerM;
}

const CARD_VARIANTS: Record<string, { border: string; text: string }> = {
  hit:  { border: "!border-ok/20", text: "text-ok [text-shadow:0_0_8px_rgba(116,184,122,0.3)]" },
  miss: { border: "!border-warn/15", text: "text-warn" },
  cost: { border: "", text: "text-accent" },
};

function CostCard({ icon, label, value, sub, variant, small }: {
  icon: React.ReactNode; label: string; value: string; sub?: string;
  variant?: "hit" | "miss" | "cost"; small?: boolean;
}) {
  const v = variant ? CARD_VARIANTS[variant] : null;
  return (
    <div className={`flex items-start rounded-lg bg-gradient-to-br from-white/[0.03] to-white/[0.01] border border-white/[0.06] shadow-[0_1px_3px_rgba(0,0,0,.15),inset_0_1px_0_rgba(255,255,255,.02)] relative overflow-hidden ${small ? "py-[5px] px-2 gap-1" : "py-2.5 pl-3 pr-2.5 gap-2"} ${v?.border ?? ""}`}>
      <div className="absolute top-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-white/[0.08] to-transparent pointer-events-none" />
      <span className="mt-px text-fg-faint shrink-0">{icon}</span>
      <div className="min-w-0">
        <div className="text-[9px] text-fg-faint uppercase tracking-[0.6px] font-medium">{label}</div>
        <div className={`font-bold tabular-nums font-mono tracking-[-0.3px] ${small ? "text-xs" : "text-[17px]"} ${v?.text ?? ""}`}>{value}</div>
        {sub && <div className="text-[11px] text-fg-faint mt-0.5">{sub}</div>}
      </div>
    </div>
  );
}

export function StatsPanel({ usage, perTurnUsage, turnSteps, context, model, sessionKey, toolCounts, skillCounts }: {
  usage?: WireUsage; perTurnUsage?: WireUsage | null; turnSteps?: WireUsage[]; context: ContextInfo; model?: string
  sessionKey: string; toolCounts: Record<string, number>; skillCounts: Record<string, number>;
}) {
  const turnRef = useRef(0);
  const stepRef = useRef(0);
  const turnAccumRef = useRef({ prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 });
  const perTurnRef = useRef<WireUsage | null>(null);
  const [data, setData] = useState<StoredData>(() => {
    const loaded = loadData(sessionKey);
    if (loaded.turns.length > 0) turnRef.current = loaded.turns[loaded.turns.length - 1].turn;
    if (loaded.steps.length > 0) stepRef.current = loaded.steps[loaded.steps.length - 1].step;
    return loaded;
  });
  const { turns: history, steps: stepHistory } = data;

  // 防御：若 sessionKey 未真正变化，跳过重载。避免因 props 抖动（如
  // sidebarSessions 数组引用刷新导致 useMemo 重算但未改变字符串值）而
  // 加载空数据覆写当前统计。
  const lastKeyRef = useRef(sessionKey);
  const keyChanged = lastKeyRef.current !== sessionKey;
  if (keyChanged) lastKeyRef.current = sessionKey;

  // sessionKey 变化时切换统计
  useEffect(() => {
    if (!keyChanged) return;
    const loaded = loadData(sessionKey);
    turnRef.current = loaded.turns.length > 0 ? loaded.turns[loaded.turns.length - 1].turn : 0;
    stepRef.current = loaded.steps.length > 0 ? loaded.steps[loaded.steps.length - 1].step : 0;
    turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 };
    perTurnRef.current = null;
    setData(loaded);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionKey]);

  // V5.31: 每个 usage 事件创建 StepRecord（实时）
  useEffect(() => {
    if (!turnSteps || turnSteps.length === 0) return;
    const lastStep = turnSteps[turnSteps.length - 1];
    // 去重：只有新步才记录
    setData(prev => {
      if (prev.steps.length > 0) {
        const prevStep = prev.steps[prev.steps.length - 1];
        // 相同 prompt+completion 视为同一步（去重）
        if (prevStep.prompt === lastStep.promptTokens && prevStep.completion === lastStep.completionTokens && prevStep.cacheHit === lastStep.cacheHitTokens && prevStep.cacheMiss === lastStep.cacheMissTokens) {
          return prev;
        }
      }
      stepRef.current += 1;
      const rec: StepRecord = {
        step: stepRef.current,
        prompt: lastStep.promptTokens,
        completion: lastStep.completionTokens,
        cacheHit: lastStep.cacheHitTokens,
        cacheMiss: lastStep.cacheMissTokens,
      };
      turnAccumRef.current.prompt += lastStep.promptTokens;
      turnAccumRef.current.completion += lastStep.completionTokens;
      turnAccumRef.current.cacheHit += lastStep.cacheHitTokens;
      turnAccumRef.current.cacheMiss += lastStep.cacheMissTokens;
      const next = { ...prev, steps: [...prev.steps, rec] };
      saveData(sessionKey, next);
      return next;
    });
  }, [turnSteps, sessionKey]);

  // V5.30: 轮次完成时创建 TurnRecord
  useEffect(() => {
    if (perTurnUsage != null) {
      perTurnRef.current = perTurnUsage;
      return;
    }
    const last = perTurnRef.current;
    if (last && last.totalTokens > 0) {
      turnRef.current += 1;
      const rec: TurnRecord = {
        turn: turnRef.current,
        prompt: turnAccumRef.current.prompt,
        completion: turnAccumRef.current.completion,
        cacheHit: turnAccumRef.current.cacheHit,
        cacheMiss: turnAccumRef.current.cacheMiss,
        cost: calcCost(turnAccumRef.current.cacheHit, modelPrice(model).cacheHit) + calcCost(turnAccumRef.current.cacheMiss, modelPrice(model).input) + calcCost(turnAccumRef.current.completion, modelPrice(model).output),
        totalTokens: last.totalTokens,
      };
      setData(prev => {
        const next = { ...prev, turns: [...prev.turns, rec] };
        saveData(sessionKey, next);
        return next;
      });
    }
    turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 };
    perTurnRef.current = null;
  }, [perTurnUsage]);

  const lastTurn = history[history.length - 1];
  const lastStep = stepHistory[stepHistory.length - 1];

  // 整个会话累计统计
  const sessionHit = usage?.sessionCacheHitTokens ?? 0;
  const sessionMiss = usage?.sessionCacheMissTokens ?? 0;
  const sessionPrompt = sessionHit + sessionMiss;

  const totalCost = history.reduce((s, r) => s + r.cost, 0);
  const lastTurnCost = lastTurn ? lastTurn.cost : 0;

  return (
    <div className="flex flex-col gap-[14px] p-[12px_14px] h-full overflow-y-auto bg-gradient-to-b from-white/[0.01] to-transparent">
      {/* ① 上下文窗口 */}
      {context.window > 0 && (
        <div className="flex flex-col gap-1.5">
          <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase mb-0.5"><Gauge size={11} className="inline mr-1 align-middle" />上下文 ({fmt(context.used)} / {fmt(context.window)})</div>
          <div className="h-2 rounded bg-border-soft overflow-hidden"><div className="h-full rounded bg-accent transition-[width] duration-300 min-w-0.5" style={{ width: `${Math.min(100, (context.used / context.window) * 100)}%` }} /></div>
        </div>
      )}

      {/* ② 会话统计 */}
      <div className="flex flex-col gap-1.5">
        <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase mb-0.5">
          <BarChart3 size={12} className="inline mr-1 align-middle" />
          会话 · {history.length}轮 · {stepHistory.length}步 · ¥{totalCost.toFixed(3)}
        </div>
        <div className="grid grid-cols-2 gap-1">
          <CostCard icon={<Hash size={11} />} label="Prompt" value={fmt(sessionPrompt)} small />
          <CostCard icon={<Zap size={11} />} label={`命中 ${pct(sessionHit, sessionPrompt)}`} value={fmt(sessionHit)} variant="hit" small />
          <CostCard icon={<BarChart3 size={11} />} label="未命中" value={fmt(sessionMiss)} variant="miss" small />
        </div>
      </div>

      {/* ③ 本轮 */}
      {lastTurn && (
        <div className="flex flex-col gap-1.5">
          <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase mb-0.5 flex items-center">
            <Hash size={12} className="inline mr-1" />
            本轮 #{lastTurn.turn}
            <span className="ml-auto text-[10px] text-fg-faint">¥{lastTurnCost.toFixed(4)}</span>
          </div>
          <div className="grid grid-cols-2 gap-1">
            <CostCard icon={<Hash size={11} />} label="Prompt" value={fmt(lastTurn.prompt)} small />
            <CostCard icon={<Zap size={11} />} label="Completion" value={fmt(lastTurn.completion)} small />
            <CostCard icon={<Zap size={11} />} label={`命中 ${pct(lastTurn.cacheHit, lastTurn.prompt)}`} value={fmt(lastTurn.cacheHit)} variant="hit" small />
            <CostCard icon={<BarChart3 size={11} />} label="未命中" value={fmt(lastTurn.cacheMiss)} variant="miss" small />
          </div>
        </div>
      )}

      {/* ④ 当前步 */}
      {lastStep && (
        <div className="flex flex-col gap-1.5">
          <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase mb-0.5 flex items-center">
            <Footprints size={12} className="inline mr-1" />
            当前步 #{lastStep.step}
          </div>
          <div className="grid grid-cols-2 gap-1">
            <CostCard icon={<Hash size={11} />} label="Prompt" value={fmt(lastStep.prompt)} small />
            <CostCard icon={<Zap size={11} />} label="Completion" value={fmt(lastStep.completion)} small />
            <CostCard icon={<Zap size={11} />} label={`命中 ${pct(lastStep.cacheHit, lastStep.prompt)}`} value={fmt(lastStep.cacheHit)} variant="hit" small />
            <CostCard icon={<BarChart3 size={11} />} label="未命中" value={fmt(lastStep.cacheMiss)} variant="miss" small />
          </div>
        </div>
      )}

      {/* ⑤ 命中率趋势（最近 20 步） */}
      {stepHistory.length > 1 && (() => {
        const recent = stepHistory.slice(-20);
        const rates = recent.map(r => r.prompt > 0 ? (r.cacheHit / r.prompt) * 100 : 0);
        const dataMin = Math.min(...rates), dataMax = Math.max(...rates), spread = dataMax - dataMin;
        const minRate = spread <= 3 ? Math.max(0, Math.floor(dataMin) - 1) : Math.max(0, Math.floor((dataMin - Math.max(5, spread * 0.15)) / 5) * 5);
        const maxRate = spread <= 3 ? 100 : Math.min(100, Math.ceil((dataMax + Math.max(5, spread * 0.15)) / 5) * 5);
        const range = maxRate - minRate || 1;
        const W = 260, H = 80, padL = 30, padR = 8, padT = 6, padB = 14;
        const plotW = W - padL - padR, plotH = H - padT - padB;
        const points = recent.map((r, i) => {
          const x = padL + (i / Math.max(1, recent.length - 1)) * plotW;
          const rate = r.prompt > 0 ? (r.cacheHit / r.prompt) * 100 : 0;
          const y = padT + plotH - ((rate - minRate) / range) * plotH;
          return { x, y, rate, step: r.step };
        });
        const path = points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
        const yTicks = [minRate, Math.round(minRate + range * 0.33), Math.round(minRate + range * 0.66), maxRate];

        return (
          <div className="flex flex-col gap-1.5">
            <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase mb-0.5">命中率趋势 (最近 {recent.length} 步)</div>
            <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto my-1">
              {yTicks.map(v => {
                const yPos = padT + plotH - ((v - minRate) / range) * plotH;
                return <text key={v} x={padL - 4} y={yPos + 3} fontSize={9} fill="var(--fg-faint)" textAnchor="end">{v}%</text>;
              })}
              {yTicks.map(v => {
                const yPos = padT + plotH - ((v - minRate) / range) * plotH;
                return <line key={"g"+v} x1={padL} y1={yPos} x2={W - padR} y2={yPos} stroke="var(--border-soft)" strokeWidth={0.5} />;
              })}
              <path d={path} fill="none" stroke="#f0a040" strokeWidth={2} strokeLinejoin="round" />
              {points.map(p => <circle key={p.step} cx={p.x} cy={p.y} r={2} fill="#f0a040"><title>步#{p.step}: {p.rate.toFixed(1)}%</title></circle>)}
              {points.filter((_, i) => i === 0 || i === points.length - 1 || i === Math.floor(points.length / 2)).map(p => (
                <text key={"x"+p.step} x={p.x} y={H - 3} fontSize={9} fill="var(--fg-faint)" textAnchor="middle">#{p.step}</text>
              ))}
            </svg>
            <div className="flex gap-3 text-[10px] text-fg-faint mt-1">
              <span><span className="inline-block w-2 h-2 rounded-sm mr-[3px] align-middle" style={{background:"#f0a040"}} /> 每步命中率</span>
            </div>
          </div>
        );
      })()}

      {/* ⑥ Token 累计趋势（最近 20 轮，按轮累计） */}
      {history.length > 1 && (() => {
        const recent = history.slice(-20);
        let cum = 0;
        const cumulative = recent.map(r => { cum += r.prompt; return cum; });
        const cumMax = Math.max(...cumulative, 1);
        const W = 260, H = 80, padL = 34, padR = 34, padT = 6, padB = 14;
        const plotW = W - padL - padR, plotH = H - padT - padB;
        const points = cumulative.map((v, i) => ({
          x: padL + (i / Math.max(1, cumulative.length - 1)) * plotW,
          y: padT + plotH - (v / cumMax) * plotH,
          v, turn: recent[i].turn
        }));
        const path = points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
        const ticks = [0, Math.round(cumMax * 0.33), Math.round(cumMax * 0.66), cumMax];

        return (
          <div className="flex flex-col gap-1.5">
            <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase mb-0.5">Token 累计趋势 (最近 {recent.length} 轮)</div>
            <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto my-1">
              {[0, 0.33, 0.66, 1].map(pct => (
                <line key={"g"+pct} x1={padL} y1={padT + plotH - pct * plotH} x2={W - padR} y2={padT + plotH - pct * plotH} stroke="var(--border-soft)" strokeWidth={0.5} />
              ))}
              {ticks.map((v, i) => {
                const pct = [0, 0.33, 0.66, 1][i];
                return <text key={"t"+i} x={padL - 4} y={padT + plotH - pct * plotH + 3} fontSize={9} fill="var(--fg-faint)" textAnchor="end">{fmt(v)}</text>;
              })}
              <path d={path} fill="none" stroke="#4ecb71" strokeWidth={1.5} />
              {points.map(p => <circle key={"p"+p.turn} cx={p.x} cy={p.y} r={1.8} fill="#4ecb71"><title>轮#{p.turn}: {fmt(p.v)} tok</title></circle>)}
            </svg>
            <div className="flex gap-3 text-[10px] text-fg-faint mt-1"><span><span className="inline-block w-2 h-2 rounded-sm mr-[3px] align-middle" style={{background:"#4ecb71"}} />按轮累计 Prompt Tokens</span></div>
          </div>
        );
      })()}

      {/* ⑦ 工具/技能统计 */}
      {(Object.keys(toolCounts).length > 0 || Object.keys(skillCounts).length > 0) && (
        <div className="flex flex-col gap-1.5">
          <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase mb-0.5"><BarChart3 size={12} className="inline mr-1 align-middle" />工具使用统计</div>
          {Object.keys(toolCounts).length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {Object.entries(toolCounts).sort((a, b) => b[1] - a[1]).map(([name, count]) => (
                <span key={name} className="inline-flex items-center gap-1.5 bg-bg-elev border border-border-soft rounded px-2 py-0.5 text-xs"><span className="font-mono text-fg">{name}</span><span className="text-[10px] text-fg-faint tabular-nums">{count}</span></span>
              ))}
            </div>
          )}
          {Object.keys(skillCounts).length > 0 && (
            <>
              <div className="text-[11px] text-fg-faint mt-0.5 mb-0.5">技能调用</div>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(skillCounts).sort((a, b) => b[1] - a[1]).map(([name, count]) => (
                  <span key={name} className="inline-flex items-center gap-1.5 bg-accent-soft border border-accent/20 rounded px-2 py-0.5 text-xs"><span className="font-mono text-accent">{name}</span><span className="text-[10px] text-accent/70 tabular-nums">{count}</span></span>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {/* 清空按钮 */}
      <div className="flex justify-end py-0.5">
        <button className="text-[10px] px-1.5 py-0.5 border border-border-soft rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:border-err" onClick={() => { setData({ turns: [], steps: [] }); saveData(sessionKey, { turns: [], steps: [] }); turnRef.current = 0; stepRef.current = 0; }} title="清空统计">
          清空统计
        </button>
      </div>
    </div>
  );
}
