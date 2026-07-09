import { memo, useState } from "react";
import { Cpu, Wallet, Coins, GitBranch, Zap } from "lucide-react";
import { Tooltip } from "./Tooltip";
import { useI18n } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import type { BalanceInfo, JobView, WireUsage } from "../lib/types";
import { priceFor, calcCost, fmtTokens, fmtCost } from "../lib/stats";


// ─── Jobs popover ─────────────────────────────────────────────────

function JobsChip({ jobs, compact }: { jobs: JobView[]; compact: boolean }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return (
    <div className="relative inline-flex">
      <button
        className={`inline-flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded text-fg-dim hover:text-fg hover:bg-bg-elev transition-colors duration-150 focus-visible:ring-1 focus-visible:ring-accent/40 focus-visible:outline-none active:scale-[0.97]`}
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

// ─── ContextBar ─────────────────────────────────────────────────

export function ContextBar({ label, used, window: win, color }: { label: string; used: number; window: number; color: string }) {
  const pct = win > 0 ? Math.round((used / win) * 100) : 0;
  const barColor = pct > 80 ? "bg-err" : pct > 60 ? "bg-warning" : color;
  return (
    <div className="flex items-center gap-1">
      <span className="text-fg-faint text-[10px] shrink-0 w-6">{label}</span>
      <div className="flex-1 h-1.5 bg-border/40 rounded-full overflow-hidden min-w-[60px]">
        <div className={`h-full rounded-full transition-all duration-500 ${barColor}`}
          style={{ width: `${Math.min(pct, 100)}%` }} />
      </div>
      <span className="text-fg-dim font-mono tabular-nums text-[10px] shrink-0 w-7 text-right">{pct}%</span>
      <span className="text-fg-faint font-mono tabular-nums text-[10px] shrink-0">{fmtTokens(used)}/{fmtTokens(win)}</span>
    </div>
  );
}

// ─── StatusBar ──────────────────────────────────────────────────

export const StatusBar = memo(function StatusBar({
  usage, balance, jobs, running, permLevel, sessionTotal = 0, bridgeAlive = true, model, subagentModel,
}: {
  usage?: WireUsage;
  balance?: BalanceInfo;
  jobs?: JobView[];
  running: boolean;
  permLevel?: string;
  sessionTotal?: number;
  bridgeAlive?: boolean;
  model?: string;
  subagentModel?: string;
  onOpenStats?: () => void;
}) {
  const compact = useCompact();

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
  const fontSize = "text-[11px]";

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

      {/* ── 模型标签 ── */}
      <div className="flex items-center gap-1 shrink-0">
        {model && (
          <Tooltip label={`主模型: ${model}`}>
            <span className="flex items-center gap-1 text-[10px] text-fg-dim font-mono bg-bg-elev border border-border-soft rounded px-1.5 py-px">
              <Cpu size={10} className="text-accent/70" />
              {model.replace("deepseek-v4-", "").replace("mimo-v2.5-", "")}
            </span>
          </Tooltip>
        )}
        {subagentModel && subagentModel !== model && (
          <Tooltip label={`子代理: ${subagentModel}`}>
            <span className="flex items-center gap-1 text-[10px] text-fg-dim font-mono bg-bg-elev border border-warn/20 rounded px-1.5 py-px">
              <GitBranch size={10} className="text-warning/70" />
              {subagentModel.replace("deepseek-v4-", "").replace("mimo-v2.5-", "")}
            </span>
          </Tooltip>
        )}
      </div>

      {/* ── 中: 运行指标 ── */}
      <div className="flex items-center gap-1.5 flex-1 min-w-0">
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
      </div>

      {/* ── 右: badge 区 ── */}
      <div className="flex items-center gap-1.5 shrink-0">
        {/* 权限级别 badge */}
        {permLevel && permLevel !== "ask" && (() => {
          const isYolo = permLevel === "yolo";
          const label = isYolo ? <><Zap size={10} className="inline" /> YOLO</> : "自动";
          const desc = isYolo ? "跳过所有确认提示" : "写入无需确认";
          const colorClass = isYolo
            ? "text-err bg-err/10 border-err/20"
            : "text-ok bg-ok/10 border-ok/20";
          return (
            <Tooltip label={desc}>
              <span className={`${fontSize} px-1.5 py-px rounded border font-medium ${colorClass}`}>
                {label}
              </span>
            </Tooltip>
          );
        })()}
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
