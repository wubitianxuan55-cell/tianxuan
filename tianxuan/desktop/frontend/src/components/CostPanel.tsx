import { useEffect, useRef, useState } from "react";
import { BarChart3, Zap, Coins, Hash, Gauge } from "lucide-react";
import type { WireUsage, ContextInfo } from "../lib/types";

const STORAGE_KEY = "tianxuan.turnHistory";

function loadHistory(): TurnRecord[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) as TurnRecord[] : [];
  } catch { return []; }
}

function saveHistory(recs: TurnRecord[]) {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(recs.slice(-200))); } catch {}
}

interface TurnRecord {
  turn: number;
  prompt: number;
  completion: number;
  cacheHit: number;
  cacheMiss: number;
  cost: number;
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

// DeepSeek 定价（每 1M token，人民币）
// V5.30: 与 StatsPanel 统一使用 CNY 价格（USD × 7.25）
const MODEL_PRICES: Record<string, { cacheHit: number; input: number; output: number; label: string }> = {
  "deepseek-v4-flash": { cacheHit: 0.0203, input: 1.015, output: 2.03, label: "V4 Flash" },
  "deepseek-v4-pro":   { cacheHit: 0.0263, input: 3.154, output: 6.308, label: "V4 Pro" },
};
const DEFAULT_PRICE = MODEL_PRICES["deepseek-v4-pro"];

function modelPrice(label?: string) {
  if (!label) return DEFAULT_PRICE;
  const lower = label.toLowerCase();
  for (const [key, p] of Object.entries(MODEL_PRICES)) {
    if (lower.includes(key)) return p;
  }
  if (lower.includes("pro")) return MODEL_PRICES["deepseek-v4-pro"];
  if (lower.includes("flash")) return MODEL_PRICES["deepseek-v4-flash"];
  return DEFAULT_PRICE;
}

function calcCost(tokens: number, pricePerM: number): number {
  return (tokens / 1_000_000) * pricePerM;
}

export function CostPanel({ usage, context, model }: { usage?: WireUsage; context: ContextInfo; model?: string }) {
  const turnRef = useRef(0);
  const lastUsageRef = useRef<WireUsage | null>(null);
  const [history, setHistory] = useState<TurnRecord[]>(() => {
    const loaded = loadHistory();
    if (loaded.length > 0) turnRef.current = loaded[loaded.length - 1].turn;
    return loaded;
  });

  // Accumulate per-turn history whenever usage changes and turn completes
  useEffect(() => {
    if (!usage || usage.totalTokens === 0) return;
    // Only record when totalTokens changes (new turn completed)
    if (lastUsageRef.current && lastUsageRef.current.totalTokens === usage.totalTokens) return;
    lastUsageRef.current = usage;
    turnRef.current += 1;
    const rec: TurnRecord = {
      turn: turnRef.current,
      prompt: usage.promptTokens,
      completion: usage.completionTokens,
      cacheHit: usage.cacheHitTokens,
      cacheMiss: usage.cacheMissTokens,
      cost: usage.costUsd ?? 0,
    };
    setHistory(prev => {
      const next = [...prev.slice(-199), rec]; // keep last 200 turns
      saveHistory(next);
      return next;
    });
  }, [usage]);

  const price = modelPrice(model);
  // 累计统计：优先从 localStorage history 累加（跨重启持久），后端计数器重启归零
  const historyHit = history.reduce((s, r) => s + r.cacheHit, 0);
  const historyMiss = history.reduce((s, r) => s + r.cacheMiss, 0);
  const backendHit = usage?.sessionCacheHitTokens ?? 0;
  const backendMiss = usage?.sessionCacheMissTokens ?? 0;
  const sessionHit = Math.max(historyHit, backendHit);
  const sessionMiss = Math.max(historyMiss, backendMiss);
  const sessionTotalPrompt = sessionHit + sessionMiss;
  const hitRate = sessionTotalPrompt > 0 ? (sessionHit / sessionTotalPrompt * 100) : 0;

  // Sum stats
  const last = history[history.length - 1];
  const promptSum = history.reduce((s, r) => s + r.prompt, 0);
  const compSum = history.reduce((s, r) => s + r.completion, 0);

  // Cost: prefer backend costUsd if available, fall back to model-based calculation
  const costHit = calcCost(sessionHit, price.cacheHit);
  const costMiss = calcCost(sessionMiss, price.input);
  const costOut = calcCost(compSum, price.output);
  const costTotal = costHit + costMiss + costOut;
  const lastCost = last ? calcCost(last.cacheHit, price.cacheHit) + calcCost(last.cacheMiss, price.input) + calcCost(last.completion, price.output) : 0;

  return (
    <div className="cost-panel">
      {/* 汇总卡片 */}
      <div className="cost-cards">
        <div className="cost-card">
          <Hash size={14} />
          <div className="cost-card__body">
            <div className="cost-card__label">会话 Token</div>
            <div className="cost-card__value">{fmt(sessionTotalPrompt)}</div>
          </div>
        </div>
        <div className="cost-card cost-card--hit">
          <Zap size={14} />
          <div className="cost-card__body">
            <div className="cost-card__label">缓存命中</div>
            <div className="cost-card__value">{fmt(sessionHit)}</div>
            <div className="cost-card__sub">{pct(sessionHit, sessionTotalPrompt)}</div>
          </div>
        </div>
        <div className="cost-card cost-card--miss">
          <BarChart3 size={14} />
          <div className="cost-card__body">
            <div className="cost-card__label">缓存未命中</div>
            <div className="cost-card__value">{fmt(sessionMiss)}</div>
            <div className="cost-card__sub">{pct(sessionMiss, sessionTotalPrompt)}</div>
          </div>
        </div>
        <div className="cost-card cost-card--cost">
          <Coins size={14} />
          <div className="cost-card__body">
            <div className="cost-card__label">预估费用</div>
            <div className="cost-card__value">¥{costTotal.toFixed(4)}</div>
            <div className="cost-card__sub">{price.label}</div>
          </div>
        </div>
      </div>

      {/* 上下文窗口 */}
      {context.window > 0 && (
        <div className="cost-section">
          <div className="cost-section__title">
            <Gauge size={11} style={{ marginRight: 4, verticalAlign: "middle" }} />
            上下文 ({fmt(context.used)} / {fmt(context.window)})
          </div>
          <div className="cost-ctx-bar">
            <div
              className="cost-ctx-bar__fill"
              style={{ width: `${Math.min(100, (context.used / context.window) * 100)}%` }}
            />
          </div>
          <div className="cost-ctx-scale">
            <span>0</span>
            <span>{fmt(Math.round(context.window / 4))}</span>
            <span>{fmt(Math.round(context.window / 2))}</span>
            <span>{fmt(Math.round(context.window * 3 / 4))}</span>
            <span>{fmt(context.window)}</span>
          </div>
        </div>
      )}

      {/* 费用明细 */}
      <div className="cost-section">
        <div className="cost-section__title">费用明细 ({price.label})</div>
        <div className="cost-row">
          <span>输入 (缓存命中)</span>
          <span className="cost-hit">¥{costHit.toFixed(4)}</span>
        </div>
        <div className="cost-row">
          <span>输入 (未命中)</span>
          <span className="cost-miss">¥{costMiss.toFixed(4)}</span>
        </div>
        <div className="cost-row">
          <span>输出</span>
          <span>¥{costOut.toFixed(4)}</span>
        </div>
        <div className="cost-row" style={{ fontWeight: 600, borderTop: "1px solid var(--border-soft)", marginTop: 2, paddingTop: 4 }}>
          <span>合计</span>
          <span>¥{costTotal.toFixed(4)}</span>
        </div>
      </div>

      {/* 本轮 */}
      {last && (
        <div className="cost-section">
          <div className="cost-section__title">本轮 (#{last.turn})</div>
          <div className="cost-row">
            <span>Prompt</span>
            <span>{fmt(last.prompt)}</span>
          </div>
          <div className="cost-row">
            <span>Completion</span>
            <span>{fmt(last.completion)}</span>
          </div>
          <div className="cost-row">
            <span>缓存命中</span>
            <span className="cost-hit">{fmt(last.cacheHit)} ({pct(last.cacheHit, last.prompt)})</span>
          </div>
          <div className="cost-row">
            <span>缓存未命中</span>
            <span className="cost-miss">{fmt(last.cacheMiss)} ({pct(last.cacheMiss, last.prompt)})</span>
          </div>
          <div className="cost-row">
            <span>费用</span>
            <span>¥{lastCost.toFixed(6)}</span>
          </div>
        </div>
      )}

      {/* 历史流水（折线图，最近 20 轮） */}
      {history.length > 1 && (() => {
        const recent = history.slice(-20);
        const W = 300, H = 120, padL = 34, padR = 10, padT = 8, padB = 18;
        const plotW = W - padL - padR, plotH = H - padT - padB;
        const rates = recent.map(r => r.prompt > 0 ? (r.cacheHit / r.prompt) * 100 : 0);
        const dataMin = Math.min(...rates);
        const dataMax = Math.max(...rates);
        const spread = dataMax - dataMin;
        // 窄区间（≤3%）用 1% 粒度，允许纵轴缩放到 97%-100%
        const minRate = spread <= 3 ? Math.max(0, Math.floor(dataMin) - 1) : Math.max(90, Math.floor(dataMin / 5) * 5);
        const maxRate = spread <= 3 ? 100 : Math.min(100, Math.ceil(dataMax / 5) * 5);
        const range = maxRate - minRate || 1;
        const points = recent.map((r, i) => {
          const x = padL + (i / Math.max(1, recent.length - 1)) * plotW;
          const rate = r.prompt > 0 ? (r.cacheHit / r.prompt) * 100 : 0;
          const y = padT + plotH - ((rate - minRate) / range) * plotH;
          return { x, y, rate, turn: r.turn };
        });
        const pathD = points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
        const yTicks = [minRate, Math.round(minRate + range * 0.33), Math.round(minRate + range * 0.66), maxRate];
        return (
          <div className="cost-section">
            <div className="cost-section__title">命中率趋势 (最近 {recent.length} 轮)</div>
            <svg viewBox={`0 0 ${W} ${H}`} className="cost-linechart">
              {/* Y axis ticks */}
              {yTicks.map(v => {
                const yPos = padT + plotH - ((v - minRate) / range) * plotH;
                return <text key={v} x={padL - 4} y={yPos + 3} className="cost-linechart__tick">{v}%</text>;
              })}
              {/* Grid lines */}
              {yTicks.map(v => {
                const yPos = padT + plotH - ((v - minRate) / range) * plotH;
                return <line key={"g"+v} x1={padL} y1={yPos} x2={W - padR} y2={yPos} className="cost-linechart__grid" />;
              })}
              {/* Hit rate line */}
              <path d={pathD} className="cost-linechart__line" fill="none" />
              {/* Dots */}
              {points.map(p => (
                <circle key={p.turn} cx={p.x} cy={p.y} r={2.5} className="cost-linechart__dot">
                  <title>#{p.turn}: {p.rate.toFixed(2)}%</title>
                </circle>
              ))}
              {/* X labels */}
              {points.filter((_, i) => i === 0 || i === points.length - 1 || i === Math.floor(points.length / 2)).map(p => (
                <text key={"x"+p.turn} x={p.x} y={H - 3} className="cost-linechart__tick">#{p.turn}</text>
              ))}
            </svg>
            <div className="cost-legend">
              <span><span className="cost-legend__dot cost-legend__dot--hit" /> 命中率</span>
            </div>
          </div>
        );
      })()}

      {/* 汇总 */}
      <div className="cost-section">
        <div className="cost-section__title">
          会话汇总
          <button className="cost-clear-btn" onClick={() => { setHistory([]); saveHistory([]); turnRef.current = 0; }} title="清空统计">
            清空
          </button>
        </div>
        <div className="cost-row">
          <span>总 Prompt</span>
          <span>{fmt(promptSum)}</span>
        </div>
        <div className="cost-row">
          <span>总 Completion</span>
          <span>{fmt(compSum)}</span>
        </div>
        <div className="cost-row">
          <span>会话命中率</span>
          <span className="cost-hit">{hitRate.toFixed(2)}%</span>
        </div>
        <div className="cost-row">
          <span>总轮数</span>
          <span>{history.length}</span>
        </div>
      </div>
    </div>
  );
}
