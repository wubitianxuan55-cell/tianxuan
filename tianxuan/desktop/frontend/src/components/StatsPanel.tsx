import { useEffect, useMemo, useRef, useState } from "react";
import { BarChart3, Gauge, TrendingUp, Zap } from "lucide-react";
import type { WireUsage, ContextInfo } from "../lib/types";

interface Point { x: number; y: number; label: string; }

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
  source?: string;
}

function tk(n: number): string { return n.toLocaleString(); }
function cash(v: number): string { return "¥" + v.toFixed(4); }

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

function hitRateColor(rate: number): string {
  return rate >= 80 ? "text-ok" : rate >= 50 ? "text-warning" : "text-err";
}
function hitRateRing(rate: number): string {
  return rate >= 80 ? "border-ok" : rate >= 50 ? "border-warning" : "border-err";
}

// ─── aggregated step stats ──────────────────────────────
interface ColStats { prompt: number; completion: number; cacheHit: number; cacheMiss: number; cost: number; }
function aggCol(steps: StepRecord[], price: ReturnType<typeof modelPrice>): ColStats {
  let prompt = 0, completion = 0, cacheHit = 0, cacheMiss = 0;
  for (const s of steps) { prompt += s.prompt; completion += s.completion; cacheHit += s.cacheHit; cacheMiss += s.cacheMiss; }
  const cost = calcCost(cacheHit, price.cacheHit) + calcCost(cacheMiss, price.input) + calcCost(completion, price.output);
  return { prompt, completion, cacheHit, cacheMiss, cost };
}

function colFromUsage(u: WireUsage | undefined, price: ReturnType<typeof modelPrice>): ColStats {
  if (!u) return { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0, cost: 0 };
  const cost = calcCost(u.cacheHitTokens, price.cacheHit) + calcCost(u.cacheMissTokens, price.input) + calcCost(u.completionTokens, price.output);
  return { prompt: u.promptTokens, completion: u.completionTokens, cacheHit: u.cacheHitTokens, cacheMiss: u.cacheMissTokens, cost };
}

// ─── 统计表格 ─────────────────────────────────────────────
function StatsTable({ title, planner, executor, sub, total }: {
  title: string; planner: ColStats; executor: ColStats; sub: ColStats; total: ColStats;
}) {
  const rows: { label: string; render: (c: ColStats) => string }[] = [
    { label: "Prompt", render: c => tk(c.prompt) },
    { label: "Compl", render: c => tk(c.completion) },
    { label: "缓存命中", render: c => {
      const t = c.cacheHit + c.cacheMiss;
      const rate = t > 0 ? (c.cacheHit / t * 100) : 0;
      return `${rate.toFixed(2)}%`;
    }},
    { label: "成本", render: c => cash(c.cost) },
  ];
  const totalHit = total.cacheHit + total.cacheMiss;
  const totalRate = totalHit > 0 ? (total.cacheHit / totalHit * 100) : 0;
  return (
    <div className="py-3 border-b border-border-soft">
      <table className="w-full text-[11px] border-collapse">
        <thead>
          <tr className="text-fg-faint border-b border-border-soft">
            <th className="text-left font-semibold pb-1 text-[10px] uppercase tracking-wider text-fg-faint" style={{width:"28%"}}>{title}</th>
            <th className="text-right font-normal pb-1" style={{width:"24%"}}>规划</th>
            <th className="text-right font-normal pb-1" style={{width:"24%"}}>执行</th>
            <th className="text-right font-normal pb-1" style={{width:"24%"}}>子代理</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const isHitRow = row.label === "缓存命中";
            const pRate = isHitRow && planner.cacheHit + planner.cacheMiss > 0 ? (planner.cacheHit / (planner.cacheHit + planner.cacheMiss) * 100) : 0;
            const eRate = isHitRow && executor.cacheHit + executor.cacheMiss > 0 ? (executor.cacheHit / (executor.cacheHit + executor.cacheMiss) * 100) : 0;
            const sRate = isHitRow && sub.cacheHit + sub.cacheMiss > 0 ? (sub.cacheHit / (sub.cacheHit + sub.cacheMiss) * 100) : 0;
            return (
              <tr key={row.label} className="border-b border-border-soft/50">
                <td className="py-1 text-fg-dim">{row.label}</td>
                {isHitRow ? (
                  <>
                    <td className={`py-1 text-right font-mono tabular-nums font-bold ${hitRateColor(pRate)}`}>
                      {planner.cacheHit + planner.cacheMiss > 0 ? `${pRate.toFixed(2)}%` : "—"}
                    </td>
                    <td className={`py-1 text-right font-mono tabular-nums font-bold ${hitRateColor(eRate)}`}>
                      {executor.cacheHit + executor.cacheMiss > 0 ? `${eRate.toFixed(2)}%` : "—"}
                    </td>
                    <td className={`py-1 text-right font-mono tabular-nums font-bold ${hitRateColor(sRate)}`}>
                      {sub.cacheHit + sub.cacheMiss > 0 ? `${sRate.toFixed(2)}%` : "—"}
                    </td>
                  </>
                ) : (
                  <>
                    <td className="py-1 text-right font-mono tabular-nums text-fg">{row.render(planner)}</td>
                    <td className="py-1 text-right font-mono tabular-nums text-fg">{row.render(executor)}</td>
                    <td className="py-1 text-right font-mono tabular-nums text-fg">{row.render(sub)}</td>
                  </>
                )}
              </tr>
            );
          })}
        </tbody>
      </table>
      {/* 汇总行 — 表格下方独立展示 */}
      <div className="mt-1.5 pt-1.5 border-t border-border-soft/70">
        <div className="flex items-center gap-2 text-[10px]">
          <span className="text-fg-faint font-semibold shrink-0">汇总</span>
          <span className="text-fg-faint select-none">·</span>
          <span className="font-mono tabular-nums text-fg">Prompt {tk(total.prompt)}</span>
          <span className="text-border select-none">·</span>
          <span className="font-mono tabular-nums text-fg">Compl {tk(total.completion)}</span>
          <span className="text-border select-none">·</span>
          <span className="font-mono tabular-nums text-fg font-semibold">{cash(total.cost)}</span>
        </div>
        {totalHit > 0 && (
          <div className="flex items-baseline gap-2 mt-0.5">
            <span className={`text-xl font-bold tabular-nums ${hitRateColor(totalRate)}`}>{totalRate.toFixed(2)}%</span>
            <span className="text-[10px] text-fg-faint tabular-nums">{tk(total.cacheHit)} 命中 / {tk(total.cacheMiss)} 未命中</span>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── 微型面积图 ─────────────────────────────────────────
function MiniAreaChart({
  title, W, H, padL, padR, padT, padB, points, yTicks, color, xLabels,
}: {
  title: string; W: number; H: number; padL: number; padR: number; padT: number; padB: number;
  points: Point[]; yTicks: [number, string][]; color: string;
  xLabels: { at: number; text: string }[];
}) {
  const plotH = H - padT - padB;
  const path = points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
  const last = points[points.length - 1];
  const first = points[0];
  const areaPath = `${path} L${last.x.toFixed(1)},${padT + plotH} L${first.x.toFixed(1)},${padT + plotH} Z`;
  return (
    <div className="py-3 border-b border-border-soft">
      <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mb-2">{title}</div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto">
        {yTicks.map(([_val, label], i) => {
          const y = padT + plotH - (i / (yTicks.length - 1)) * plotH;
          return (
            <g key={`y${i}`}>
              <line x1={padL} y1={y} x2={W - padR} y2={y} stroke="var(--border-soft)" strokeWidth={0.5} />
              <text x={padL - 4} y={y + 3} fontSize={9} fill="var(--fg-faint)" textAnchor="end">{label}</text>
            </g>
          );
        })}
        <path d={areaPath} fill={color} opacity={0.08} />
        <path d={path} fill="none" stroke={color} strokeWidth={2} strokeLinejoin="round" />
        {points.map((p, i) => (
          <circle key={i} cx={p.x} cy={p.y} r={2} fill={color}>
            <title>{p.label}</title>
          </circle>
        ))}
        {xLabels.map((xl, i) => (
          <text key={i} x={xl.at} y={H - 3} fontSize={9} fill="var(--fg-faint)" textAnchor="middle">{xl.text}</text>
        ))}
      </svg>
    </div>
  );
}

// ─── 命中率趋势图（从 StepRecord[] 生成）─────────────────
function HitRateTrend({ steps, title, color }: { steps: StepRecord[]; title: string; color: string }) {
  const recent = steps.slice(-20);
  if (recent.length < 2) return null;
  const rates = recent.map(r => r.prompt > 0 ? (r.cacheHit / r.prompt) * 100 : 0);
  const dataMin = Math.min(...rates), dataMax = Math.max(...rates), spread = dataMax - dataMin || 1;
  let step: number, padding: number;
  if (spread <= 0.5)    { step = 0.1; padding = 0.05; }
  else if (spread <= 1) { step = 0.2; padding = 0.1; }
  else if (spread <= 2) { step = 1;   padding = 0.5; }
  else if (spread <= 5) { step = 2;   padding = 1.0; }
  else                  { step = 5;   padding = Math.max(5, spread * 0.15); }
  const minRate = Math.max(0, Math.floor((dataMin - padding) / step) * step);
  const maxRate = Math.min(100, Math.ceil((dataMax + padding) / step) * step);
  const range = Math.max(maxRate - minRate || 1, step);
  const W = 260, H = 80, padL = 30, padR = 8, padT = 8, padB = 16;
  const plotW = W - padL - padR, plotH = H - padT - padB;
  const points: Point[] = recent.map((r, i) => {
    const x = padL + (i / Math.max(1, recent.length - 1)) * plotW;
    const rate = r.prompt > 0 ? (r.cacheHit / r.prompt) * 100 : 0;
    const y = padT + plotH - ((rate - minRate) / range) * plotH;
    return { x, y, label: `步#${r.step}: ${rate.toFixed(2)}%` };
  });
  const fmt = (v: number) => step < 1 ? v.toFixed(1) + "%" : `${Math.round(v)}%`;
  const mid = minRate + range * 0.5;
  const yLabels: [number, string][] = [[minRate, fmt(minRate)]];
  if (mid !== minRate && mid !== maxRate) yLabels.push([mid, fmt(mid)]);
  if (maxRate !== minRate) yLabels.push([maxRate, fmt(maxRate)]);
  const xLabels = [
    { at: points[0].x, text: `#${recent[0].step}` },
    ...(recent.length >= 3 ? [{ at: points[Math.floor(points.length / 2)].x, text: `#${recent[Math.floor(recent.length / 2)].step}` }] : []),
    { at: points[points.length - 1].x, text: `#${recent[recent.length - 1].step}` },
  ];
  return <MiniAreaChart title={title} W={W} H={H} padL={padL} padR={padR} padT={padT} padB={padB} points={points} yTicks={yLabels} color={color} xLabels={xLabels} />;
}


export function StatsPanel({ usage, perTurnUsage, turnSteps, context, model, subagentModel, plannerModel, sessionKey, resetKey, toolCounts, skillCounts, perTurnPlannerUsage, perTurnExecutorUsage, perTurnSubUsage }: {
  usage?: WireUsage; perTurnUsage?: WireUsage | null; turnSteps?: WireUsage[]; context: ContextInfo; model?: string; subagentModel?: string; plannerModel?: string;
  sessionKey: string; resetKey?: number; toolCounts: Record<string, number>; skillCounts: Record<string, number>;
  perTurnPlannerUsage?: WireUsage; perTurnExecutorUsage?: WireUsage; perTurnSubUsage?: WireUsage;
}) {
  const turnRef = useRef(0);
  const stepRef = useRef(0);
  const turnAccumRef = useRef<{ prompt: number; completion: number; cacheHit: number; cacheMiss: number }>({ prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 });
  const perTurnRef = useRef<WireUsage | null>(null);
  const [data, setData] = useState<StoredData>(() => {
    const loaded = loadData(sessionKey);
    if (loaded.turns.length > 0) turnRef.current = loaded.turns[loaded.turns.length - 1].turn;
    if (loaded.steps.length > 0) stepRef.current = loaded.steps[loaded.steps.length - 1].step;
    return loaded;
  });
  const { turns: history, steps: stepHistory } = data;

  const lastKeyRef = useRef(sessionKey);
  const keyChanged = lastKeyRef.current !== sessionKey;
  if (keyChanged) lastKeyRef.current = sessionKey;

  const lastResetRef = useRef(resetKey);
  const skipWriteRef = useRef(false);
  useEffect(() => {
    if (resetKey === undefined || resetKey === lastResetRef.current) return;
    lastResetRef.current = resetKey;
    skipWriteRef.current = true;
    saveData(sessionKey, { turns: [], steps: [] });
    turnRef.current = 0; stepRef.current = 0;
    turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 };
    perTurnRef.current = null;
    setData({ turns: [], steps: [] });
  }, [resetKey, sessionKey]);
  useEffect(() => {
    if (!keyChanged) return;
    const loaded = loadData(sessionKey);
    turnRef.current = loaded.turns.length > 0 ? loaded.turns[loaded.turns.length - 1].turn : 0;
    stepRef.current = loaded.steps.length > 0 ? loaded.steps[loaded.steps.length - 1].step : 0;
    turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 };
    perTurnRef.current = null;
    setData(loaded);
  }, [sessionKey]);

  useEffect(() => {
    if (!turnSteps || turnSteps.length === 0) return;
    if (skipWriteRef.current) { skipWriteRef.current = false; return; }
    const lastStep = turnSteps[turnSteps.length - 1];
    setData(prev => {
      if (prev.steps.length > 0) {
        const prevStep = prev.steps[prev.steps.length - 1];
        if (prevStep.prompt === lastStep.promptTokens && prevStep.completion === lastStep.completionTokens
          && prevStep.cacheHit === lastStep.cacheHitTokens && prevStep.cacheMiss === lastStep.cacheMissTokens) {
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
        source: lastStep.source,
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
    if (perTurnUsage != null) { perTurnRef.current = perTurnUsage; return; }
    const last = perTurnRef.current;
    if (last && last.totalTokens > 0) {
      turnRef.current += 1;
      const rec: TurnRecord = {
        turn: turnRef.current,
        prompt: turnAccumRef.current.prompt,
        completion: turnAccumRef.current.completion,
        cacheHit: turnAccumRef.current.cacheHit,
        cacheMiss: turnAccumRef.current.cacheMiss,
        cost: calcCost(turnAccumRef.current.cacheHit, modelPrice(model).cacheHit)
          + calcCost(turnAccumRef.current.cacheMiss, modelPrice(model).input)
          + calcCost(turnAccumRef.current.completion, modelPrice(model).output),
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

  // ── stats computation ──────────────────────────────────
  const price = modelPrice(model);           // executor model price
  const plannerPrice = modelPrice(plannerModel); // planner model price
  const subPrice = modelPrice(subagentModel); // subagent model price (V10.31: use subagent's own price)
  const sessionHit = usage?.sessionCacheHitTokens ?? 0;
  const sessionMiss = usage?.sessionCacheMissTokens ?? 0;
  const sessionPrompt = sessionHit + sessionMiss;
  const sessionRate = sessionPrompt > 0 ? (sessionHit / sessionPrompt * 100) : 0;

  // session-level: aggregate from localStorage stepHistory, split by source
  const plannerSteps = stepHistory.filter(s => s.source === "planner");
  const executorSteps = stepHistory.filter(s => s.source !== "subagent" && s.source !== "planner"); // "main" or legacy
  const subSteps = stepHistory.filter(s => s.source === "subagent");
  const sessPlanner = useMemo(() => aggCol(plannerSteps, plannerPrice), [plannerSteps, plannerPrice]);
  const sessExecutor = useMemo(() => aggCol(executorSteps, price), [executorSteps, price]);
  const sessSub = useMemo(() => aggCol(subSteps, subPrice), [subSteps, subPrice]);
  const sessTotal = useMemo(() => ({
    prompt: sessPlanner.prompt + sessExecutor.prompt + sessSub.prompt,
    completion: sessPlanner.completion + sessExecutor.completion + sessSub.completion,
    cacheHit: sessPlanner.cacheHit + sessExecutor.cacheHit + sessSub.cacheHit,
    cacheMiss: sessPlanner.cacheMiss + sessExecutor.cacheMiss + sessSub.cacheMiss,
    cost: sessPlanner.cost + sessExecutor.cost + sessSub.cost,
  }), [sessPlanner, sessExecutor, sessSub]);
  const totalCost = sessPlanner.cost + sessExecutor.cost + sessSub.cost;

  // turn-level: from store accumulators
  const turnPlanner = useMemo(() => colFromUsage(perTurnPlannerUsage, plannerPrice), [perTurnPlannerUsage, plannerPrice]);
  const turnExecutor = useMemo(() => colFromUsage(perTurnExecutorUsage, price), [perTurnExecutorUsage, price]);
  const turnSub = useMemo(() => colFromUsage(perTurnSubUsage, subPrice), [perTurnSubUsage, subPrice]);
  const turnTotal = useMemo(() => ({
    prompt: turnPlanner.prompt + turnExecutor.prompt + turnSub.prompt,
    completion: turnPlanner.completion + turnExecutor.completion + turnSub.completion,
    cacheHit: turnPlanner.cacheHit + turnExecutor.cacheHit + turnSub.cacheHit,
    cacheMiss: turnPlanner.cacheMiss + turnExecutor.cacheMiss + turnSub.cacheMiss,
    cost: turnPlanner.cost + turnExecutor.cost + turnSub.cost,
  }), [turnPlanner, turnExecutor, turnSub]);

  const lastStep = stepHistory[stepHistory.length - 1];
  const hasAnyData = history.length > 0 || stepHistory.length > 0;

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      {/* ── 顶栏摘要 ── */}
      <div className="flex items-center gap-2 px-3 py-2 bg-bg-soft border-b border-border-soft shrink-0">
        <span className={`inline-block w-3 h-3 rounded-full border-2 ${hitRateRing(sessionRate)}`} />
        {sessionPrompt > 0 ? (
          <span className={`text-[11px] font-bold tabular-nums ${hitRateColor(sessionRate)}`}>{sessionRate.toFixed(2)}%</span>
        ) : (
          <span className="text-[11px] text-fg-faint">—</span>
        )}
        {context.window > 0 && (
          <>
            <span className="text-border select-none">·</span>
            <Gauge size={11} className="text-fg-faint" />
            <span className="text-[10px] text-fg-faint font-mono tabular-nums">{tk(context.used)}/{tk(context.window)}</span>
          </>
        )}
        <span className="text-border select-none">·</span>
        <span className="text-[10px] text-fg-faint font-mono tabular-nums">
          {history.length}轮·{stepHistory.length}步
        </span>
        {totalCost > 0 && (
          <>
            <span className="text-border select-none">·</span>
            <span className="text-[10px] text-fg-faint font-mono tabular-nums">{cash(totalCost)}</span>
          </>
        )}
      </div>

      {!hasAnyData ? (
        <div className="flex flex-col items-center justify-center gap-2 flex-1 text-fg-faint">
          <BarChart3 size={32} className="opacity-30" />
          <span className="text-[12px]">暂无统计数据</span>
          <span className="text-[10px] opacity-60">发起对话后自动开始记录</span>
        </div>
      ) : (
      <div className="flex flex-col gap-0 p-3 overflow-y-auto">

        {/* ── 会话级统计表格 ── */}
        <StatsTable
          title={`会话 (${history.length}轮·${stepHistory.length}步)`}
          planner={sessPlanner} executor={sessExecutor} sub={sessSub} total={sessTotal}
        />

        {/* ── 本轮级统计表格 ── */}

        {(perTurnPlannerUsage || perTurnExecutorUsage || perTurnSubUsage) && (
          <StatsTable title={`本轮 (${turnSteps?.length || 0}步)`} planner={turnPlanner} executor={turnExecutor} sub={turnSub} total={turnTotal} />
        )}

        {/* ── 当前步 ── */}
        {lastStep && (
          <div className="py-3 border-b border-border-soft">
            <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mb-2">
              当前步 #{lastStep.step}
              {lastStep.source && (
                <span className={`ml-2 text-[9px] px-1 rounded ${lastStep.source === "subagent" ? "bg-warn-soft text-warning" : lastStep.source === "planner" ? "bg-accent-soft/50 text-accent/80" : "bg-accent-soft text-accent"}`}>
                  {lastStep.source === "subagent" ? "子代理" : lastStep.source === "planner" ? "规划模型" : "执行模型"}
                </span>
              )}
            </div>
            <div className="flex items-center gap-1 text-[11px] text-fg-dim font-mono tabular-nums mb-1.5">
              <span>Prompt {tk(lastStep.prompt)}</span>
              <span className="text-border mx-1.5">·</span>
              <span>Compl {tk(lastStep.completion)}</span>
            </div>
            {lastStep.prompt > 0 && (() => {
              const rate = (lastStep.cacheHit / lastStep.prompt) * 100;
              return (
                <div className="flex items-baseline gap-2">
                  <span className={`text-xl font-bold tabular-nums ${hitRateColor(rate)}`}>{rate.toFixed(2)}%</span>
                  <span className="text-[10px] text-fg-faint tabular-nums">{tk(lastStep.cacheHit)} 命中 / {tk(lastStep.cacheMiss)} 未命中</span>
                </div>
              );
            })()}
          </div>
        )}

        {/* ── 命中率趋势（规划）── */}
        <HitRateTrend steps={plannerSteps} title={`命中率趋势 · ${plannerModel || "规划模型"}`} color="var(--accent)" />

        {/* ── 命中率趋势（执行）── */}
        <HitRateTrend steps={executorSteps} title={`命中率趋势 · ${model || "执行模型"}`} color="#3b82f6" />

        {/* ── 命中率趋势（子代理）── */}
        <HitRateTrend steps={subSteps} title={`命中率趋势 · ${subagentModel || "子代理"}`} color="var(--warn)" />

        {/* ── Token 趋势 ── */}
        {history.length > 1 && (() => {
          const recent = history.slice(-20);
          let cumP = 0, cumC = 0;
          const pCumulative: number[] = []; const cCumulative: number[] = [];
          for (const r of recent) { cumP += r.prompt; cumC += r.completion; pCumulative.push(cumP); cCumulative.push(cumC); }
          const pMax = Math.max(...pCumulative, 1); const cMax = Math.max(...cCumulative, 1);
          const W = 260, H = 88, padL = 40, padR = 40, padT = 6, padB = 18;
          const plotW = W - padL - padR, plotH = H - padT - padB;
          const pToY = (v: number) => padT + plotH - (v / pMax) * plotH;
          const cToY = (v: number) => padT + plotH - (v / cMax) * plotH;
          const pPoints: Point[] = pCumulative.map((v, i) => ({
            x: padL + (i / Math.max(1, pCumulative.length - 1)) * plotW, y: pToY(v),
            label: `轮#${recent[i].turn}: Prompt ${tk(v)}`,
          }));
          const cPoints: Point[] = cCumulative.map((v, i) => ({
            x: padL + (i / Math.max(1, cCumulative.length - 1)) * plotW, y: cToY(v),
            label: `轮#${recent[i].turn}: Compl ${tk(v)}`,
          }));
          const pPath = pPoints.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
          const cPath = cPoints.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
          const tkK = (n: number) => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + "K" : String(n);
          const pYLabels: [number, string][] = [[0, "0"], [Math.round(pMax * 0.5), tkK(Math.round(pMax * 0.5))], [pMax, tkK(pMax)]];
          const cYLabels: [number, string][] = [[0, "0"], [Math.round(cMax * 0.5), tk(Math.round(cMax * 0.5))], [cMax, tk(cMax)]];
          const xLabels = recent.length >= 3 ? [{ at: pPoints[0].x, text: `#${recent[0].turn}` }, { at: pPoints[pPoints.length - 1].x, text: `#${recent[recent.length - 1].turn}` }] : [];
          return (
            <div className="py-3 border-b border-border-soft">
              <div className="flex items-center justify-between mb-2">
                <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider">
                  <TrendingUp size={11} className="inline mr-1 align-middle" />
                  Token 趋势 (最近 {recent.length} 轮)
                </div>
                <div className="flex items-center gap-3 text-[9px] text-fg-faint">
                  <span className="flex items-center gap-1"><span className="w-2 h-0.5 rounded" style={{background:"#22d3ee"}} />输入</span>
                  <span className="flex items-center gap-1"><span className="w-2 h-0.5 rounded" style={{background:"#fb923c"}} />输出</span>
                </div>
              </div>
              <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto">
                {pYLabels.map(([_val, label], i) => {
                  const y = padT + plotH - (i / (pYLabels.length - 1)) * plotH;
                  return (<g key={`py${i}`}><line x1={padL} y1={y} x2={W - padR} y2={y} stroke="var(--border-soft)" strokeWidth={0.5} /><text x={padL - 4} y={y + 3} fontSize={9} fill="#22d3ee" textAnchor="end">{label}</text></g>);
                })}
                {cYLabels.map(([_val, label], i) => {
                  const y = padT + plotH - (i / (cYLabels.length - 1)) * plotH;
                  return (<g key={`cy${i}`}><text x={W - padR + 4} y={y + 3} fontSize={9} fill="#fb923c" textAnchor="start">{label}</text></g>);
                })}
                <path d={pPath} fill="none" stroke="#22d3ee" strokeWidth={2} strokeLinejoin="round" />
                {pPoints.map((p, i) => (<circle key={"p"+i} cx={p.x} cy={p.y} r={2} fill="#22d3ee"><title>{p.label}</title></circle>))}
                <path d={cPath} fill="none" stroke="#fb923c" strokeWidth={2} strokeLinejoin="round" />
                {cPoints.map((p, i) => (<circle key={"c"+i} cx={p.x} cy={p.y} r={2} fill="#fb923c"><title>{p.label}</title></circle>))}
                {xLabels.map((xl, i) => (<text key={i} x={xl.at} y={H - 3} fontSize={9} fill="var(--fg-faint)" textAnchor="middle">{xl.text}</text>))}
              </svg>
            </div>
          );
        })()}

        {/* ── 工具 / 技能统计 ── */}
        {(Object.keys(toolCounts).length > 0 || Object.keys(skillCounts).length > 0) && (
          <div className="py-3">
            {Object.keys(toolCounts).length > 0 && (
              <>
                <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mb-2">
                  <Zap size={12} className="inline mr-1 align-middle" /> 工具调用
                </div>
                <div className="flex flex-wrap gap-1">
                  {Object.entries(toolCounts).sort((a, b) => b[1] - a[1]).map(([name, count]) => (
                    <span key={name} className="inline-flex items-center gap-1 bg-bg-elev border border-border-soft rounded px-1.5 py-0.5 text-[11px]">
                      <span className="font-mono text-fg">{name}</span>
                      <span className="text-[10px] text-fg-faint tabular-nums">{count}</span>
                    </span>
                  ))}
                </div>
              </>
            )}
            {Object.keys(skillCounts).length > 0 && (
              <>
                <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mb-2 mt-2.5">
                  <BarChart3 size={12} className="inline mr-1 align-middle" /> 技能调用
                </div>
                <div className="flex flex-wrap gap-1">
                  {Object.entries(skillCounts).sort((a, b) => b[1] - a[1]).map(([name, count]) => (
                    <span key={name} className="inline-flex items-center gap-1 bg-accent-soft border border-accent/20 rounded px-1.5 py-0.5 text-[11px]">
                      <span className="font-mono text-accent">{name}</span>
                      <span className="text-[10px] text-accent/70 tabular-nums">{count}</span>
                    </span>
                  ))}
                </div>
              </>
            )}
          </div>
        )}

        {/* 清空按钮 */}
        <div className="flex justify-end pb-1">
          <button className="text-[10px] px-1.5 py-0.5 border border-border-soft rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:border-err transition-colors" onClick={() => { setData({ turns: [], steps: [] }); saveData(sessionKey, { turns: [], steps: [] }); turnRef.current = 0; stepRef.current = 0; }} title="清空统计">
            清空统计
          </button>
        </div>
      </div>
      )}
    </div>
  );
}
