import { memo, useEffect, useState } from "react";
import { Cpu, Wallet, Coins } from "lucide-react";
import { Tooltip } from "./Tooltip";
import { useI18n } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import type { BalanceInfo, ContextInfo, JobView, WireUsage } from "../lib/types";

// ─── 模型价格表（与 StatsPanel 共用逻辑）─────────────────────────

const MODEL_PRICES: Record<string, { cacheHit: number; input: number; output: number }> = {
  "deepseek-v4-flash": { cacheHit: 0.0203, input: 1.015, output: 2.03 },
  "deepseek-v4-pro":   { cacheHit: 0.0263, input: 3.154, output: 6.308 },
};
const DEFAULT_PRICE = MODEL_PRICES["deepseek-v4-flash"];

function priceFor(label?: string) {
  if (!label) return DEFAULT_PRICE;
  for (const [key, p] of Object.entries(MODEL_PRICES)) {
    if (label.includes(key)) return p;
  }
  return DEFAULT_PRICE;
}

function calcCost(tokens: number, pricePerM: number): number {
  return (tokens / 1_000_000) * pricePerM;
}

// ─── 格式化 ───────────────────────────────────────────────────────

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  return String(n);
}

function fmtCost(v: number): string {
  if (v >= 0.01) return "¥" + v.toFixed(2);
  if (v > 0) return "¥" + v.toFixed(4);
  return "¥0";
}

function fmtElapsed(ms: number): string {
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m${s % 60}s`;
}

function useTick(on: boolean): number {
  const [, setN] = useState(0);
  useEffect(() => {
    if (!on) return;
    const id = setInterval(() => setN((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, [on]);
  return Date.now();
}

// ─── Jobs popover ─────────────────────────────────────────────────

function JobsChip({ jobs, compact }: { jobs: JobView[]; compact: boolean }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return (
    <div className="relative inline-flex">
      <button
        className={`inline-flex items-center gap-1 ${compact ? "text-[11px]" : "text-[11px]"} px-1.5 py-0.5 rounded text-fg-dim hover:text-fg hover:bg-bg-elev transition-colors`}
        onClick={() => setOpen((v) => !v)}
        title={t("status.jobsTitle")}
      >
        <Cpu size={compact ? 11 : 12} />
        <span>任务</span>
        <span className="font-mono tabular-nums">{jobs.length}</span>
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute bottom-full left-0 mb-1 z-50 bg-bg-elev-2 border border-border rounded-lg min-w-[200px] max-h-[240px] overflow-y-auto" style={{boxShadow: "var(--ds-shadow-dropdown)"}}>
            <div className="px-3 py-2 text-[11px] text-fg-faint font-medium border-b border-border-soft">{t("status.jobsTitle")}</div>
            {jobs.map((j) => (
              <div className="flex items-center gap-2 px-3 py-1.5 text-[12px] hover:bg-bg-elev border-b border-border-soft last:border-0" key={j.id} role="option">
                <span className="text-fg-faint text-[11px] font-mono">{j.id}</span>
                <span className="flex-1 truncate text-fg-dim">{j.label || j.kind}</span>
                <span className="text-[11px] text-fg-faint">{j.status}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// ─── StatusBar ──────────────────────────────────────────────────

export const StatusBar = memo(function StatusBar({
  context, usage, balance, jobs, running, agentMode, yolo, turnStartAt, turnTokens, sessionTotal = 0, bridgeAlive = true, model,
}: {
  context: ContextInfo;
  usage?: WireUsage;
  balance?: BalanceInfo;
  jobs?: JobView[];
  running: boolean;
  agentMode?: string;
  yolo?: boolean;
  turnStartAt: number;
  turnTokens: number;
  sessionTotal?: number;
  bridgeAlive?: boolean;
  model?: string;
  onOpenStats?: () => void;
}) {
  const compact = useCompact();
  const now = useTick(running);

  const elapsedMs = running && turnStartAt ? Math.max(0, now - turnStartAt) : 0;
  const elapsed = elapsedMs > 0 ? fmtElapsed(elapsedMs) : "";
  const tokLabel = running && turnTokens > 0 ? fmtTokens(turnTokens) : "";

  // ── 会话成本 ──
  const p = priceFor(model);
  const sessionHit = usage?.sessionCacheHitTokens ?? 0;
  const sessionMiss = usage?.sessionCacheMissTokens ?? 0;
  const sessionCompl = usage?.completionTokens ?? 0;
  const totalPromptTokens = sessionHit + sessionMiss;
  const sessionCost = totalPromptTokens > 0
    ? calcCost(sessionHit, p.cacheHit) + calcCost(sessionMiss, p.input) + calcCost(sessionCompl, p.output)
    : 0;

  // ── 缓存率 ──
  const totalCacheable = sessionHit + sessionMiss;
  const sessionRate = totalCacheable > 0 ? Math.round((sessionHit / totalCacheable) * 100) : 0;
  const nowDenom = (usage?.cacheHitTokens ?? 0) + (usage?.cacheMissTokens ?? 0);
  const nowRate = nowDenom > 0 ? Math.round(((usage?.cacheHitTokens ?? 0) / nowDenom) * 100) : null;
  const cacheColor = sessionRate >= 80 ? "text-ok" : sessionRate >= 50 ? "text-warning" : "text-err";
  const cacheBadge = sessionRate >= 80 ? "bg-ok/10 border-ok/20" : sessionRate >= 50 ? "bg-warning/10 border-warning/20" : "bg-err/10 border-err/20";
  const nowColor = nowRate == null ? "" : nowRate >= 80 ? "text-ok" : nowRate >= 50 ? "text-warning" : "text-err";

  const barH = compact ? "h-7" : "h-8";
  const barPx = compact ? "px-2" : "px-3";
  const fontSize = compact ? "text-[11px]" : "text-[11.5px]";
  const contextPct = context.window > 0 ? Math.round((context.used / context.window) * 100) : 0;
  const contextColor = contextPct > 80 ? "bg-err" : contextPct > 60 ? "bg-warning" : "bg-accent";

  const connLabel = !bridgeAlive ? "离线" : running ? "生成中" : "在线";
  const connColor = !bridgeAlive ? "bg-err" : running ? "bg-warning ds-pulse" : "bg-ok";
  const connTextColor = !bridgeAlive ? "text-err" : running ? "text-warning" : "text-ok";

  return (
    <div className={`flex items-center gap-2 ${barH} ${barPx} ${fontSize} bg-bg-soft border-t border-border-soft select-none shrink-0`} data-wails-no-drag>
      {/* ── 左: 连接灯 + 状态文字 ── */}
      <div className="flex items-center gap-1.5 shrink-0">
        <Tooltip label={connLabel}>
          <span className={`w-2 h-2 rounded-full block ${connColor}`} />
        </Tooltip>
        <span className={`${connTextColor} font-medium`}>{connLabel}</span>
      </div>

      <span className="text-border/30 select-none">│</span>

      {/* ── 中: 运行指标 ── */}
      <div className="flex items-center gap-1.5 flex-1 min-w-0">
        {running ? (
          <>
            <Tooltip label="本轮耗时">
              <span className="text-fg-dim tabular-nums font-mono">{elapsed}</span>
            </Tooltip>
            {tokLabel && (
              <>
                <span className="text-border/40 select-none">·</span>
                <Tooltip label="本轮 Token">
                  <span className="text-fg-dim tabular-nums font-mono">↓{tokLabel}</span>
                </Tooltip>
              </>
            )}
          </>
        ) : (
          <>
            {sessionTotal > 0 && (
              <>
                <Tooltip label="会话 Token 总量">
                  <span className="text-fg-dim font-mono tabular-nums">
                    {fmtTokens(sessionTotal)} tk
                  </span>
                </Tooltip>
                <span className="text-border/40 select-none">·</span>
                <Tooltip label={`会话费用 ¥${sessionCost.toFixed(4)}`}>
                  <span className="text-fg-dim font-mono tabular-nums flex items-center gap-0.5">
                    <Coins size={compact ? 11 : 12} className="text-fg-faint" />
                    <span className="text-fg-faint">费</span>
                    {fmtCost(sessionCost)}
                  </span>
                </Tooltip>
              </>
            )}
            {/* 上下文用量 — 加宽带标签 */}
            {context.window > 0 && (
              <>
                <span className="text-border/40 select-none">·</span>
                <Tooltip label={`上下文: ${fmtTokens(context.used)} / ${fmtTokens(context.window)}`}>
                  <div className="flex items-center gap-1">
                    <span className="text-fg-faint">上下文</span>
                    <div className="w-[60px] h-[5px] bg-border rounded-full overflow-hidden">
                      <div
                        className={`h-full rounded-full transition-all duration-300 ${contextColor}`}
                        style={{ width: `${Math.min(contextPct, 100)}%` } as React.CSSProperties}
                      />
                    </div>
                    <span className="text-fg-faint font-mono tabular-nums text-[9px]">{contextPct}%</span>
                  </div>
                </Tooltip>
              </>
            )}
          </>
        )}
      </div>

      {/* ── 右: badge 区 ── */}
      <div className="flex items-center gap-1.5 shrink-0">
        {/* 统一模式 badge */}
        {agentMode && agentMode !== "" && (
          <span className={`${fontSize} px-1.5 py-px rounded border font-medium ${
            agentMode === "explore" ? "text-info bg-info/10 border-info/20" :
            agentMode === "orchestrate" ? "text-accent bg-accent-soft border-accent/30" :
            "text-ok bg-ok/10 border-ok/20"
          }`}>
            {agentMode === "explore" ? "探索" : agentMode === "develop" ? "开发" : "编排"}
          </span>
        )}

        {/* YOLO badge */}
        {yolo && (
          <Tooltip label="YOLO：自动批准所有工具">
            <span className="ds-chip ds-chip--accent">YOLO</span>
          </Tooltip>
        )}

        {/* 缓存详情 — 始终显示，带文字标注 */}
        {usage && sessionTotal > 0 && (
          <>
            <span className="text-border/40 select-none">│</span>
            <Tooltip label={nowRate != null ? `本轮 ${nowRate}% · 会话 ${sessionRate}%` : `会话命中率 ${sessionRate}%`}>
              <span className={`${fontSize} font-semibold px-1.5 py-px rounded border ${cacheColor} ${cacheBadge}`}>
                缓存 {sessionRate}%
                {nowRate != null && <span className={`ml-0.5 ${nowColor}`}>·{nowRate}%</span>}
              </span>
            </Tooltip>
            <Tooltip label="提示 Token">
              <span className="text-fg-dim tabular-nums">提{fmtTokens(usage?.promptTokens ?? 0)}</span>
            </Tooltip>
            <Tooltip label="输出 Token">
              <span className="text-fg-dim tabular-nums">出{fmtTokens(usage?.completionTokens ?? 0)}</span>
            </Tooltip>
            <Tooltip label="缓存命中">
              <span className="text-ok tabular-nums">✓{fmtTokens(usage?.cacheHitTokens ?? 0)}</span>
            </Tooltip>
            <Tooltip label="缓存未命中">
              <span className="text-err tabular-nums">✗{fmtTokens(usage?.cacheMissTokens ?? 0)}</span>
            </Tooltip>
          </>
        )}

        {/* 后台任务 — 文字标注 */}
        {jobs && jobs.length > 0 && <JobsChip jobs={jobs} compact={compact} />}

        {/* 余额 — 文字标注 */}
        {balance?.available && balance.display && (
          <Tooltip label="账户余额">
            <span className="inline-flex items-center gap-1 text-fg-dim tabular-nums">
              <Wallet size={compact ? 11 : 12} />
              <span className="text-fg-faint">余额</span>
              <span className="font-mono">{balance.display}</span>
            </span>
          </Tooltip>
        )}
      </div>
    </div>
  );
});
