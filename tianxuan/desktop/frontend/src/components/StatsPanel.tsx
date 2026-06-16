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

  // sessionKey 变化时切换统计
  useEffect(() => {
    const loaded = loadData(sessionKey);
    turnRef.current = loaded.turns.length > 0 ? loaded.turns[loaded.turns.length - 1].turn : 0;
    stepRef.current = loaded.steps.length > 0 ? loaded.steps[loaded.steps.length - 1].step : 0;
    turnAccumRef.current = { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0 };
    perTurnRef.current = null;
    setData(loaded);
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
    <div className="cost-panel">
      {/* ① 上下文窗口 */}
      {context.window > 0 && (
        <div className="cost-section">
          <div className="cost-section__title"><Gauge size={11} style={{ marginRight: 4, verticalAlign: "middle" }} />上下文 ({fmt(context.used)} / {fmt(context.window)})</div>
          <div className="cost-ctx-bar"><div className="cost-ctx-bar__fill" style={{ width: `${Math.min(100, (context.used / context.window) * 100)}%` }} /></div>
        </div>
      )}

      {/* ② 会话统计 */}
      <div className="cost-section">
        <div className="cost-section__title">
          <BarChart3 size={12} style={{ marginRight: 4, verticalAlign: "middle" }} />
          会话 · {history.length}轮 · {stepHistory.length}步 · ¥{totalCost.toFixed(3)}
        </div>
        <div className="cost-cards" style={{ gap: 4 }}>
          <div className="cost-card" style={{ padding: "5px 8px", gap: 4 }}>
            <Hash size={11} /><div className="cost-card__body"><div className="cost-card__label">Prompt</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(sessionPrompt)}</div></div>
          </div>
          <div className="cost-card cost-card--hit" style={{ padding: "5px 8px", gap: 4 }}>
            <Zap size={11} /><div className="cost-card__body"><div className="cost-card__label">命中 {pct(sessionHit, sessionPrompt)}</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(sessionHit)}</div></div>
          </div>
          <div className="cost-card cost-card--miss" style={{ padding: "5px 8px", gap: 4 }}>
            <BarChart3 size={11} /><div className="cost-card__body"><div className="cost-card__label">未命中</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(sessionMiss)}</div></div>
          </div>
        </div>
      </div>

      {/* ③ 本轮 */}
      {lastTurn && (
        <div className="cost-section">
          <div className="cost-section__title" style={{ marginBottom: 4 }}>
            <Hash size={12} style={{ marginRight: 4, verticalAlign: "middle" }} />
            本轮 #{lastTurn.turn}
            <span style={{ marginLeft: "auto", fontSize: 10, color: "var(--fg-faint)" }}>¥{lastTurnCost.toFixed(4)}</span>
          </div>
          <div className="cost-cards" style={{ gap: 4 }}>
            <div className="cost-card" style={{ padding: "5px 8px", gap: 4 }}>
              <Hash size={11} /><div className="cost-card__body"><div className="cost-card__label">Prompt</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastTurn.prompt)}</div></div>
            </div>
            <div className="cost-card" style={{ padding: "5px 8px", gap: 4 }}>
              <Zap size={11} /><div className="cost-card__body"><div className="cost-card__label">Completion</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastTurn.completion)}</div></div>
            </div>
            <div className="cost-card cost-card--hit" style={{ padding: "5px 8px", gap: 4 }}>
              <Zap size={11} /><div className="cost-card__body"><div className="cost-card__label">命中 {pct(lastTurn.cacheHit, lastTurn.prompt)}</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastTurn.cacheHit)}</div></div>
            </div>
            <div className="cost-card cost-card--miss" style={{ padding: "5px 8px", gap: 4 }}>
              <BarChart3 size={11} /><div className="cost-card__body"><div className="cost-card__label">未命中</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastTurn.cacheMiss)}</div></div>
            </div>
          </div>
        </div>
      )}

      {/* ④ 当前步 */}
      {lastStep && (
        <div className="cost-section">
          <div className="cost-section__title" style={{ marginBottom: 4 }}>
            <Footprints size={12} style={{ marginRight: 4, verticalAlign: "middle" }} />
            当前步 #{lastStep.step}
          </div>
          <div className="cost-cards" style={{ gap: 4 }}>
            <div className="cost-card" style={{ padding: "5px 8px", gap: 4 }}>
              <Hash size={11} /><div className="cost-card__body"><div className="cost-card__label">Prompt</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastStep.prompt)}</div></div>
            </div>
            <div className="cost-card" style={{ padding: "5px 8px", gap: 4 }}>
              <Zap size={11} /><div className="cost-card__body"><div className="cost-card__label">Completion</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastStep.completion)}</div></div>
            </div>
            <div className="cost-card cost-card--hit" style={{ padding: "5px 8px", gap: 4 }}>
              <Zap size={11} /><div className="cost-card__body"><div className="cost-card__label">命中 {pct(lastStep.cacheHit, lastStep.prompt)}</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastStep.cacheHit)}</div></div>
            </div>
            <div className="cost-card cost-card--miss" style={{ padding: "5px 8px", gap: 4 }}>
              <BarChart3 size={11} /><div className="cost-card__body"><div className="cost-card__label">未命中</div><div className="cost-card__value" style={{ fontSize: 12 }}>{fmt(lastStep.cacheMiss)}</div></div>
            </div>
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
          <div className="cost-section">
            <div className="cost-section__title">命中率趋势 (最近 {recent.length} 步)</div>
            <svg viewBox={`0 0 ${W} ${H}`} className="cost-linechart">
              {yTicks.map(v => {
                const yPos = padT + plotH - ((v - minRate) / range) * plotH;
                return <text key={v} x={padL - 4} y={yPos + 3} className="cost-linechart__tick">{v}%</text>;
              })}
              {yTicks.map(v => {
                const yPos = padT + plotH - ((v - minRate) / range) * plotH;
                return <line key={"g"+v} x1={padL} y1={yPos} x2={W - padR} y2={yPos} className="cost-linechart__grid" />;
              })}
              <path d={path} fill="none" stroke="#f0a040" strokeWidth={2} strokeLinejoin="round" />
              {points.map(p => <circle key={p.step} cx={p.x} cy={p.y} r={2} fill="#f0a040"><title>步#{p.step}: {p.rate.toFixed(1)}%</title></circle>)}
              {points.filter((_, i) => i === 0 || i === points.length - 1 || i === Math.floor(points.length / 2)).map(p => (
                <text key={"x"+p.step} x={p.x} y={H - 3} className="cost-linechart__tick">#{p.step}</text>
              ))}
            </svg>
            <div className="cost-legend">
              <span><span className="cost-legend__dot" style={{background:"#f0a040"}} /> 每步命中率</span>
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
          <div className="cost-section">
            <div className="cost-section__title">Token 累计趋势 (最近 {recent.length} 轮)</div>
            <svg viewBox={`0 0 ${W} ${H}`} className="cost-linechart">
              {[0, 0.33, 0.66, 1].map(pct => (
                <line key={"g"+pct} x1={padL} y1={padT + plotH - pct * plotH} x2={W - padR} y2={padT + plotH - pct * plotH} className="cost-linechart__grid" />
              ))}
              {ticks.map((v, i) => {
                const pct = [0, 0.33, 0.66, 1][i];
                return <text key={"t"+i} x={padL - 4} y={padT + plotH - pct * plotH + 3} className="cost-linechart__tick" textAnchor="end">{fmt(v)}</text>;
              })}
              <path d={path} fill="none" stroke="#4ecb71" strokeWidth={1.5} />
              {points.map(p => <circle key={"p"+p.turn} cx={p.x} cy={p.y} r={1.8} fill="#4ecb71"><title>轮#{p.turn}: {fmt(p.v)} tok</title></circle>)}
            </svg>
            <div className="cost-legend"><span><span className="cost-legend__dot" style={{background:"#4ecb71"}} />按轮累计 Prompt Tokens</span></div>
          </div>
        );
      })()}

      {/* ⑦ 工具/技能统计 */}
      {(Object.keys(toolCounts).length > 0 || Object.keys(skillCounts).length > 0) && (
        <div className="cost-section">
          <div className="cost-section__title"><BarChart3 size={12} style={{ marginRight: 4, verticalAlign: "middle" }} />工具使用统计</div>
          {Object.keys(toolCounts).length > 0 && (
            <div className="cost-tags">
              {Object.entries(toolCounts).sort((a, b) => b[1] - a[1]).map(([name, count]) => (
                <span key={name} className="cost-tag"><span className="cost-tag__name">{name}</span><span className="cost-tag__count">{count}</span></span>
              ))}
            </div>
          )}
          {Object.keys(skillCounts).length > 0 && (
            <>
              <div style={{ fontSize: 11, color: "var(--fg-faint)", margin: "4px 0 2px" }}>技能调用</div>
              <div className="cost-tags">
                {Object.entries(skillCounts).sort((a, b) => b[1] - a[1]).map(([name, count]) => (
                  <span key={name} className="cost-tag cost-tag--skill"><span className="cost-tag__name">{name}</span><span className="cost-tag__count">{count}</span></span>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {/* 清空按钮 */}
      <div className="cost-section" style={{ textAlign: "right", padding: "2px 0" }}>
        <button className="cost-clear-btn" onClick={() => { setData({ turns: [], steps: [] }); saveData(sessionKey, { turns: [], steps: [] }); turnRef.current = 0; stepRef.current = 0; }} title="清空统计">
          清空统计
        </button>
      </div>
    </div>
  );
}
