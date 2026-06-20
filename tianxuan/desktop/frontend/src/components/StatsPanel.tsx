import { useEffect, useRef, useState } from "react";
import { BarChart3 } from "lucide-react";
import type { WireUsage, ContextInfo } from "../lib/types";

function storageKey(sessionKey: string) { return `tianxuan.stats.${sessionKey}`; }

interface StoredData { turns: TurnRecord[]; steps: StepRecord[]; }

function loadData(sessionKey: string): StoredData {
  try {
    const raw = localStorage.getItem(storageKey(sessionKey));
    if (!raw) return { turns: [], steps: [] };
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) return { turns: parsed as TurnRecord[], steps: [] };
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

function SectionCard({ title, detail, prompt, completion, hit, miss, hitPct, cost }: {
  title: string; detail?: string; prompt: string; completion: string;
  hit: string; miss: string; hitPct: string; cost?: string;
}) {
  const hitNum = parseFloat(hitPct);
  const hitColor = !isNaN(hitNum) ? hitNum >= 80 ? "text-ok" : hitNum >= 50 ? "text-warning" : "text-err" : "text-fg-faint";
  return (
    <div className="border border-border-soft rounded-lg bg-bg-elev/40 overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 bg-bg-elev/60 border-b border-border-soft">
        <span className="text-[11px] font-semibold text-fg">{title}</span>
        {detail && <span className="text-[10px] text-fg-faint">{detail}</span>}
      </div>
      <div className="flex items-stretch">
        <div className="flex-1 grid grid-cols-2 gap-y-1 px-3 py-2 text-[11px]">
          <span className="text-fg-faint">输入</span><span className="text-right font-mono tabular-nums text-fg">{prompt}</span>
          <span className="text-fg-faint">输出</span><span className="text-right font-mono tabular-nums text-fg">{completion}</span>
          {cost !== undefined && <><span className="text-fg-faint">成本</span><span className="text-right font-mono tabular-nums text-accent">{cost}</span></>}
        </div>
        <div className="w-px bg-border-soft" />
        <div className="flex flex-col items-center justify-center px-4 py-2 min-w-[72px]">
          <span className={`text-xl font-bold tabular-nums leading-none ${hitColor}`}>{hitPct}%</span>
          <span className="text-[10px] text-fg-faint mt-0.5">命中率</span>
          <span className="text-[10px] font-mono tabular-nums mt-0.5"><span className="text-ok">{hit}</span> / <span className="text-err">{miss}</span></span>
        </div>
      </div>
    </div>
  );
}

export function StatsPanel({ usage, perTurnUsage, turnSteps, context, model, sessionKey, toolCounts, skillCounts }: {
  usage?: WireUsage; perTurnUsage?: WireUsage | null; turnSteps?: WireUsage[]; context: ContextInfo; model?: string
  sessionKey: string; refreshNonce: number; toolCounts: Record<string, number>; skillCounts: Record<string, number>;
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

  useEffect(() => {
    const loaded = loadData(sessionKey);
    turnRef.current = loaded.turns.length > 0 ? loaded.turns[loaded.turns.length - 1].turn : 0;
    stepRef.current = loaded.steps.length > 0 ? loaded.steps[loaded.steps.length - 1].step : 0;
    turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 };
    perTurnRef.current = null;
    setData(loaded);
  }, [sessionKey]);

  useEffect(() => {
    if (!turnSteps || turnSteps.length === 0) return;
    const lastStep = turnSteps[turnSteps.length - 1];
    setData(prev => {
      if (prev.steps.length > 0) {
        const prevStep = prev.steps[prev.steps.length - 1];
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

  // ── Derived data ──
  const sessionHit = usage?.sessionCacheHitTokens ?? 0;
  const sessionMiss = usage?.sessionCacheMissTokens ?? 0;
  const sessionPrompt = sessionHit + sessionMiss;
  const sessionCompletion = history.reduce((s, r) => s + r.completion, 0) + (usage?.completionTokens ?? 0);
  const hitPct = sessionPrompt > 0 ? Math.round((sessionHit / sessionPrompt) * 100) : 0;

  const lastTurn = history[history.length - 1];
  const lastStep = stepHistory[stepHistory.length - 1];
  const totalCost = history.reduce((s, r) => s + r.cost, 0);
  const lastTurnCost = lastTurn ? lastTurn.cost : 0;

  return (
    <div className="flex flex-col gap-[14px] p-[12px_14px] h-full overflow-y-auto">

      {/* ── ① Top: cache health ring + summary ── */}
      {context.window > 0 && (
        <div className="flex items-center gap-4 px-1 pb-1">
          {/* Cache health ring */}
          <div className="relative w-11 h-11 shrink-0">
            <svg viewBox="0 0 36 36" className="w-full h-full -rotate-90">
              <circle cx="18" cy="18" r="14" fill="none" stroke="var(--border-soft)" strokeWidth="3" />
              <circle cx="18" cy="18" r="14" fill="none" stroke="#74b87a" strokeWidth="3.5"
                strokeDasharray={`${hitPct * 0.88} 88`} strokeLinecap="round" />
            </svg>
            <span className="absolute inset-0 flex items-center justify-center text-[10px] font-bold text-ok leading-none">{hitPct}%</span>
          </div>
          {/* Summary text */}
          <div className="flex flex-col min-w-0 leading-tight">
            <span className="text-[12px] text-fg font-medium truncate">{fmt(context.used)} / {fmt(context.window)} ctx</span>
            <span className="text-[10px] text-fg-faint">{history.length}轮 · {stepHistory.length}步 · ¥{totalCost.toFixed(3)}</span>
          </div>
        </div>
      )}

      {/* ── ② 会话 ── */}
      <SectionCard
        title="会话"
        detail={`${history.length}轮 · ${stepHistory.length}步`}
        prompt={sessionPrompt.toLocaleString()}
        completion={sessionCompletion.toLocaleString()}
        hit={sessionHit.toLocaleString()}
        miss={sessionMiss.toLocaleString()}
        hitPct={sessionPrompt > 0 ? (sessionHit / sessionPrompt * 100).toFixed(1) : "—"}
        cost={`¥${totalCost.toFixed(3)}`}
      />

      {/* ── ③ 本轮 ── */}
      {lastTurn && (
        <SectionCard
          title={`本轮 #${lastTurn.turn}`}
          prompt={lastTurn.prompt.toLocaleString()}
          completion={lastTurn.completion.toLocaleString()}
          hit={lastTurn.cacheHit.toLocaleString()}
          miss={lastTurn.cacheMiss.toLocaleString()}
          hitPct={lastTurn.prompt > 0 ? (lastTurn.cacheHit / lastTurn.prompt * 100).toFixed(1) : "—"}
          cost={`¥${lastTurnCost.toFixed(4)}`}
        />
      )}

      {/* ── ④ 当前步 ── */}
      {lastStep && (
        <SectionCard
          title={`当前步 #${lastStep.step}`}
          prompt={lastStep.prompt.toLocaleString()}
          completion={lastStep.completion.toLocaleString()}
          hit={lastStep.cacheHit.toLocaleString()}
          miss={lastStep.cacheMiss.toLocaleString()}
          hitPct={lastStep.prompt > 0 ? (lastStep.cacheHit / lastStep.prompt * 100).toFixed(1) : "—"}
        />
      )}

      {/* ── ⑤ Cache hit rate trend (last 20 steps) ── */}
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
            <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase">命中率趋势 (最近 {recent.length} 步)</div>
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
          </div>
        );
      })()}

      {/* ── ⑥ Token cumulative trend (last 20 turns) ── */}
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
            <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase">Token 累计趋势 (最近 {recent.length} 轮)</div>
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
          </div>
        );
      })()}

      {/* ── ⑤ Tool/skill usage badges ── */}
      {/* ── ⑤ Tool/skill usage mini bars ── */}
      {(Object.keys(toolCounts).length > 0 || Object.keys(skillCounts).length > 0) && (() => {
        const allItems = [
          ...Object.entries(toolCounts).map(([name, count]) => ({ name, count, kind: "tool" as const })),
          ...Object.entries(skillCounts).map(([name, count]) => ({ name, count, kind: "skill" as const })),
        ].sort((a, b) => b.count - a.count).slice(0, 8);
        const maxCount = allItems[0]?.count ?? 1;
        return (
          <div className="flex flex-col gap-1.5">
            <div className="text-[10px] font-semibold tracking-[0.5px] text-fg-faint uppercase"><BarChart3 size={12} className="inline mr-1 align-middle" />工具/技能</div>
            <div className="flex flex-col gap-[3px]">
              {allItems.map(item => (
                <div key={item.name} className="flex items-center gap-2 text-[10.5px]">
                  <span className={`w-16 text-right truncate font-mono shrink-0 ${item.kind === "skill" ? "text-accent" : "text-fg-faint"}`}>{item.name}</span>
                  <div className="flex-1 h-2.5 bg-border-soft rounded-sm overflow-hidden">
                    <div
                      className={`h-full rounded-sm transition-all duration-500 ${item.kind === "skill" ? "bg-accent/50" : "bg-fg-dim/25"}`}
                      style={{ width: `${Math.max(3, (item.count / maxCount) * 100)}%` }}
                    />
                  </div>
                  <span className="w-5 text-right tabular-nums text-fg-faint shrink-0">{item.count}</span>
                </div>
              ))}
            </div>
          </div>
        );
      })()}

      {/* Clear button */}
      <div className="flex justify-end py-0.5">
        <button className="text-[10px] px-1.5 py-0.5 border border-border-soft rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:border-err" onClick={() => { setData({ turns: [], steps: [] }); saveData(sessionKey, { turns: [], steps: [] }); turnRef.current = 0; stepRef.current = 0; }}>
          清空统计
        </button>
      </div>
    </div>
  );
}
