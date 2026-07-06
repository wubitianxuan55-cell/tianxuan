import { useEffect, useMemo, useRef, useState } from "react";
import { BarChart3, TrendingUp, Zap } from "lucide-react";
import type { WireUsage } from "../lib/types";
import { aggSteps, colFromUsage, hitRateColor, type StepRecord, type ColStats } from "../lib/stats";
import { TrendChart, type TrendPoint } from "./TrendChart";

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

function tk(n: number): string { return n.toLocaleString(); }
function cash(v: number): string { return "¥" + v.toFixed(4); }

// ─── useStatsPersistence ──────────────────────────────────
// 将 localStorage 持久化逻辑提取为独立 hook，在 App 层运行。
// StatsPanel 可安全条件渲染（不再需要 display:none 保活），
// 此 hook 始终运行以接收 usage 事件并写入 localStorage。

export function useStatsPersistence(
  sessionKey: string,
  resetKey: number | undefined,
  turnSteps: WireUsage[] | undefined,
  perTurnUsage: WireUsage | null | undefined,
) {
  const turnRef = useRef(0);
  const stepRef = useRef(0);
  const turnAccumRef = useRef<{ prompt: number; completion: number; cacheHit: number; cacheMiss: number; cost: number }>({ prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0, cost: 0 });
  const perTurnRef = useRef<WireUsage | null>(null);
  const [data, setData] = useState<StoredData>(() => {
    const loaded = loadData(sessionKey);
    if (loaded.turns.length > 0) turnRef.current = loaded.turns[loaded.turns.length - 1].turn;
    if (loaded.steps.length > 0) stepRef.current = loaded.steps[loaded.steps.length - 1].step;
    return loaded;
  });

  const lastKeyRef = useRef(sessionKey);
  const keyChanged = lastKeyRef.current !== sessionKey;
  if (keyChanged) lastKeyRef.current = sessionKey;

  const lastResetRef = useRef(resetKey);
  const skipWriteRef = useRef(false);
  // Merged effect: keyChanged (load data) runs before reset (clear data) to avoid
  // race when both sessionKey and resetKey change in the same render cycle.
  useEffect(() => {
    const kc = lastKeyRef.current !== sessionKey;
    if (kc) {
      lastKeyRef.current = sessionKey;
      const loaded = loadData(sessionKey);
      turnRef.current = loaded.turns.length > 0 ? loaded.turns[loaded.turns.length - 1].turn : 0;
      stepRef.current = loaded.steps.length > 0 ? loaded.steps[loaded.steps.length - 1].step : 0;
      turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0, cost: 0 };
      perTurnRef.current = null;
      setData(loaded);
    }
    if (resetKey !== undefined && resetKey !== lastResetRef.current) {
      lastResetRef.current = resetKey;
      skipWriteRef.current = true;
      saveData(sessionKey, { turns: [], steps: [] });
      turnRef.current = 0; stepRef.current = 0;
      turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0, cost: 0 };
      perTurnRef.current = null;
      setData({ turns: [], steps: [] });
    }
  }, [resetKey, sessionKey]);

  useEffect(() => {
    if (!turnSteps || turnSteps.length === 0) return;
    if (skipWriteRef.current) { skipWriteRef.current = false; return; }
    const lastStep = turnSteps[turnSteps.length - 1];
    setData(prev => {
      if (prev.steps.length > 0) {
        const prevStep = prev.steps[prev.steps.length - 1];
        if (prevStep.prompt === lastStep.promptTokens && prevStep.completion === lastStep.completionTokens
          && prevStep.cacheHit === lastStep.cacheHitTokens && prevStep.cacheMiss === lastStep.cacheMissTokens
          && prevStep.source === lastStep.source) {
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
        cost: lastStep.costUsd ?? 0,
        source: lastStep.source,
      };
      turnAccumRef.current.prompt += lastStep.promptTokens;
      turnAccumRef.current.completion += lastStep.completionTokens;
      turnAccumRef.current.cacheHit += lastStep.cacheHitTokens;
      turnAccumRef.current.cacheMiss += lastStep.cacheMissTokens;
      turnAccumRef.current.cost += lastStep.costUsd ?? 0;
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
        cost: turnAccumRef.current.cost,
        totalTokens: last.totalTokens,
      };
      setData(prev => {
        const next = { ...prev, turns: [...prev.turns, rec] };
        saveData(sessionKey, next);
        return next;
      });
    }
    turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0, cost: 0 };
    perTurnRef.current = null;
  }, [perTurnUsage]);

  const clearData = () => {
    saveData(sessionKey, { turns: [], steps: [] });
    turnRef.current = 0;
    stepRef.current = 0;
    setData({ turns: [], steps: [] });
  };

  return { data, clearData };
}

// ─── 统计表格 ─────────────────────────────────────────────
function StatsTable({ title, planner, executor, sub, total, collapsed }: {
  title: string; planner: ColStats; executor: ColStats; sub: ColStats; total: ColStats;
  collapsed?: boolean;
}) {
  // ── collapsed: summary only ──
  if (collapsed) {
    const t = total.cacheHit + total.cacheMiss;
    const rate = t > 0 ? (total.cacheHit / t) * 100 : 0;
    return (
      <div className="py-3 border-b border-border-soft">
        <table className="w-full text-[11px] border-collapse">
          <tbody>
            <tr className="font-bold">
              <td className="py-1 text-fg" style={{width:"28%"}}>{title}</td>
              <td className="py-1 text-right font-mono tabular-nums" style={{width:"24%"}}>{tk(total.prompt)}</td>
              <td className="py-1 text-right font-mono tabular-nums" style={{width:"24%"}}>{tk(total.completion)}</td>
              <td className="py-1 text-right font-mono tabular-nums" style={{width:"24%"}}>{cash(total.cost)}</td>
            </tr>
            <tr>
              <td colSpan={4} className="py-1 text-left">
                {t === 0 ? (
                  <span className="text-[10px] text-fg-faint">—</span>
                ) : (
                  <span className={`text-base font-bold tabular-nums ${hitRateColor(rate)}`}>
                    {rate.toFixed(2)}% <span className="text-[10px] text-fg-faint font-normal">{tk(total.cacheHit)} 命中 / {tk(total.cacheMiss)} 未命中</span>
                  </span>
                )}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    );
  }
  // ── expanded: full detail ──
  const rows: { label: string; render: (c: ColStats) => string }[] = [
    { label: "Prompt", render: c => tk(c.prompt) },
    { label: "Compl", render: c => tk(c.completion) },
    { label: "缓存命中", render: c => {
      const t2 = c.cacheHit + c.cacheMiss;
      const r2 = t2 > 0 ? (c.cacheHit / t2 * 100) : 0;
      return `${r2.toFixed(2)}%`;
    }},
    { label: "成本", render: c => cash(c.cost) },
  ];
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
                    <td className="py-1 text-right font-mono tabular-nums">{row.render(planner)}</td>
                    <td className="py-1 text-right font-mono tabular-nums">{row.render(executor)}</td>
                    <td className="py-1 text-right font-mono tabular-nums">{row.render(sub)}</td>
                  </>
                )}
              </tr>
            );
          })}
          <tr className="font-bold border-t-2 border-border-soft">
            <td className="py-1 text-fg">合计</td>
            <td className="py-1 text-right font-mono tabular-nums">{tk(total.prompt)}</td>
            <td className="py-1 text-right font-mono tabular-nums">{tk(total.completion)}</td>
            <td className="py-1 text-right font-mono tabular-nums">{cash(total.cost)}</td>
          </tr>
          <tr>
            <td colSpan={4} className="py-2 text-left">
              {(() => {
                const t2 = total.cacheHit + total.cacheMiss;
                if (t2 === 0) return <span className="text-[10px] text-fg-faint">—</span>;
                const rate2 = (total.cacheHit / t2) * 100;
                return (
                  <div className="flex items-baseline gap-2">
                    <span className={`text-xl font-bold tabular-nums ${hitRateColor(rate2)}`}>{rate2.toFixed(2)}%</span>
                    <span className="text-[10px] text-fg-faint tabular-nums">{tk(total.cacheHit)} 命中 / {tk(total.cacheMiss)} 未命中</span>
                  </div>
                );
              })()}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}

// ─── 命中率趋势 ──────────────────────────────────────────
function HitRateTrend({ steps, title, color, callCount }: { steps: StepRecord[]; title: string; color: string; callCount: number }) {
  const recent = steps.slice(-20);
  if (recent.length < 2) return null;
  // 命中率 = cacheHit / (cacheHit + cacheMiss)
  const rates = recent.map(r => {
    const total = r.cacheHit + r.cacheMiss;
    return total > 0 ? (r.cacheHit / total) * 100 : 0;
  });
  // 自适应 Y 轴粒度：数据范围越窄，刻度越精细
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
  const points: TrendPoint[] = recent.map((r, i) => {
    const x = padL + (i / Math.max(1, recent.length - 1)) * plotW;
    const total = r.cacheHit + r.cacheMiss;
    const rate = total > 0 ? (r.cacheHit / total) * 100 : 0;
    const y = padT + plotH - ((rate - minRate) / range) * plotH;
    return { x, y, label: `步#${r.step}: ${rate.toFixed(2)}%` };
  });
  const fmt = (v: number) => step < 1 ? v.toFixed(1) + "%" : `${Math.round(v)}%`;
  const yLabels: [number, string][] = [[minRate, fmt(minRate)]];
  const mid = minRate + range * 0.5;
  if (mid !== minRate && mid !== maxRate) yLabels.push([mid, fmt(mid)]);
  if (maxRate !== minRate) yLabels.push([maxRate, fmt(maxRate)]);
  const xLabels = [
    { at: points[0].x, text: `#${recent[0].step}` },
    ...(recent.length >= 3 ? [{ at: points[Math.floor(points.length / 2)].x, text: `#${recent[Math.floor(recent.length / 2)].step}` }] : []),
    { at: points[points.length - 1].x, text: `#${recent[recent.length - 1].step}` },
  ];
  const avgRate = rates.reduce((a, r) => a + r, 0) / rates.length;
  return (
    <div className="py-3 border-b border-border-soft">
      <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mb-1.5">
        {title} · 最近 {recent.length} 步 · {callCount}次调用 · 均值 {avgRate.toFixed(1)}%
      </div>
      <TrendChart
        W={W} H={H} padL={padL} padR={padR} padT={padT} padB={padB}
        points={points} yTicks={yLabels} color={color} xLabels={xLabels}
        fillOpacity={0.08}
      />
    </div>
  );
}


// ─── Token 趋势图 ────────────────────────────────────────
function TokenTrendChart({ history }: { history: TurnRecord[] }) {
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
}

// ─── StatsPanel ──────────────────────────────────────────
// 纯展示组件，不再管理 localStorage 持久化。
// 持久化由 App 层调用 useStatsPersistence 负责。

export function StatsPanel({ data, clearData, turnSteps, subagentModel, toolCounts, skillCounts, perTurnPlannerUsage, perTurnExecutorUsage, perTurnSubUsage }: {
  data: StoredData;
  clearData: () => void;
  turnSteps?: WireUsage[]; subagentModel?: string;
  toolCounts: Record<string, number>; skillCounts: Record<string, number>;
  perTurnPlannerUsage?: WireUsage; perTurnExecutorUsage?: WireUsage; perTurnSubUsage?: WireUsage;
}) {
  const { turns: history, steps: stepHistory } = data;
  const [sessionExpanded, setSessionExpanded] = useState(false);
  const [turnExpanded, setTurnExpanded] = useState(false);

  // ── stats computation ──────────────────────────────────

  // session-level: aggregate from localStorage stepHistory, split by source
  const plannerSteps = stepHistory.filter(s => s.source === "planner");
  const executorSteps = stepHistory.filter(s => s.source === "main" || s.source === "executor" || !s.source);
  const subSteps = stepHistory.filter(s => s.source === "subagent");
  const sessPlanner = useMemo(() => aggSteps(plannerSteps), [plannerSteps]);
  const sessExecutor = useMemo(() => aggSteps(executorSteps), [executorSteps]);
  const sessSub = useMemo(() => aggSteps(subSteps), [subSteps]);
  const sessTotal = useMemo(() => ({
    prompt: sessPlanner.prompt + sessExecutor.prompt + sessSub.prompt,
    completion: sessPlanner.completion + sessExecutor.completion + sessSub.completion,
    cacheHit: sessPlanner.cacheHit + sessExecutor.cacheHit + sessSub.cacheHit,
    cacheMiss: sessPlanner.cacheMiss + sessExecutor.cacheMiss + sessSub.cacheMiss,
    cost: sessPlanner.cost + sessExecutor.cost + sessSub.cost,
  }), [sessPlanner, sessExecutor, sessSub]);

  // turn-level: from store accumulators
  const turnPlanner = useMemo(() => colFromUsage(perTurnPlannerUsage), [perTurnPlannerUsage]);
  const turnExecutor = useMemo(() => colFromUsage(perTurnExecutorUsage), [perTurnExecutorUsage]);
  const turnSub = useMemo(() => colFromUsage(perTurnSubUsage), [perTurnSubUsage]);
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

      {!hasAnyData ? (
        <div className="flex flex-col items-center justify-center gap-2 flex-1 text-fg-faint">
          <BarChart3 size={32} className="opacity-30" />
          <span className="text-[12px]">暂无统计数据</span>
          <span className="text-[10px] opacity-60">发起对话后自动开始记录</span>
        </div>
      ) : (
      <div className="flex flex-col gap-0 p-3 overflow-y-auto">

        {/* ── 会话级统计表格 ── */}
        <div className="cursor-pointer select-none" onClick={() => setSessionExpanded(!sessionExpanded)}>
          <StatsTable
            title={`会话 (${history.length}轮·${stepHistory.length}步)`}
            planner={sessPlanner} executor={sessExecutor} sub={sessSub} total={sessTotal}
            collapsed={!sessionExpanded}
          />
        </div>
        {sessionExpanded && (<div className="text-[10px] text-fg-faint text-center -mt-2 mb-1">▲ 点击收起明细</div>)}
        {!sessionExpanded && (<div className="text-[10px] text-fg-faint text-center -mt-2 mb-1">▼ 点击展开明细</div>)}

        {/* ── 本轮级统计表格 ── */}

        {(perTurnPlannerUsage || perTurnExecutorUsage || perTurnSubUsage) && (
          <>
            <div className="cursor-pointer select-none" onClick={() => setTurnExpanded(!turnExpanded)}>
              <StatsTable title={`本轮 (${turnSteps?.length || 0}步)`} planner={turnPlanner} executor={turnExecutor} sub={turnSub} total={turnTotal} collapsed={!turnExpanded} />
            </div>
            {turnExpanded && (<div className="text-[10px] text-fg-faint text-center -mt-2 mb-1">▲ 点击收起明细</div>)}
            {!turnExpanded && (<div className="text-[10px] text-fg-faint text-center -mt-2 mb-1">▼ 点击展开明细</div>)}
          </>
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
        <HitRateTrend steps={plannerSteps} title="命中率趋势 · Hermes" color="var(--accent)" callCount={plannerSteps.length} />

        {/* ── 命中率趋势（执行）── */}
        <HitRateTrend steps={executorSteps} title="命中率趋势 · Hephaestus" color="#3b82f6" callCount={executorSteps.length} />

        {/* ── 命中率趋势（子代理）── */}
        <HitRateTrend steps={subSteps} title={`命中率趋势 · ${subagentModel || "子代理"}`} color="var(--warn)" callCount={subSteps.length} />

        {/* ── Token 趋势 ── */}
        {history.length > 1 && <TokenTrendChart history={history} />}

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
          <button className="text-[10px] px-1.5 py-0.5 border border-border-soft rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:border-err transition-colors" onClick={clearData} title="清空统计">
            清空统计
          </button>
        </div>
      </div>
      )}
    </div>
  );
}
