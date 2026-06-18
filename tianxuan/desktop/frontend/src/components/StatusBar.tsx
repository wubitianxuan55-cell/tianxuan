import { useEffect, useState } from "react";
import { Cpu, Wallet } from "lucide-react";
import { ModelSwitcher } from "./ModelSwitcher";
import { useI18n } from "../lib/i18n";
import type { BalanceInfo, ContextInfo, JobView, Meta, Mode, WireUsage } from "../lib/types";

// JobsChip — background-jobs indicator with popover.
function JobsChip({ jobs }: { jobs: JobView[] }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return (
    <div className="relative inline-flex">
      <button
        className="inline-flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded text-(--color-fg-dim) hover:text-(--color-fg) hover:bg-(--color-bg-elev) transition-colors"
        onClick={() => setOpen((v) => !v)}
        title={t("status.jobsTitle")}
      >
        <Cpu size={11} />
        {t("status.jobs", { n: jobs.length })}
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute bottom-full left-0 mb-1 z-50 bg-(--color-bg-elev-2) border border-(--color-border) rounded-lg shadow-lg min-w-[200px] max-h-[240px] overflow-y-auto" role="listbox">
            <div className="px-3 py-2 text-[11px] text-(--color-fg-faint) font-medium border-b border-(--color-border-soft)">
              {t("status.jobsTitle")}
            </div>
            {jobs.map((j) => (
              <div className="flex items-center gap-2 px-3 py-1.5 text-[12px] hover:bg-(--color-bg-elev) border-b border-(--color-border-soft) last:border-0" key={j.id} role="option">
                <span className="text-(--color-fg-faint) text-[11px] font-mono">{j.id}</span>
                <span className="flex-1 truncate text-(--color-fg-dim)">{j.label || j.kind}</span>
                <span className="text-[11px] text-(--color-fg-faint)">{j.status}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function nowRate(u?: WireUsage): number | null {
  if (!u) return null;
  let denom = u.cacheHitTokens + u.cacheMissTokens;
  if (denom === 0) denom = u.promptTokens;
  if (denom <= 0) return null;
  return Math.round((u.cacheHitTokens / denom) * 100);
}

function avgRate(u?: WireUsage): number | null {
  if (!u) return null;
  const denom = u.sessionCacheHitTokens + u.sessionCacheMissTokens;
  if (denom <= 0) return null;
  return Math.round((u.sessionCacheHitTokens / denom) * 100);
}

function fmtTokens(n: number): string {
  if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  return String(n);
}

function fmtElapsed(ms: number): string {
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m ${s % 60}s`;
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

export function StatusBar({
  meta,
  context,
  usage,
  balance,
  jobs,
  running,
  mode,
  turnStartAt,
  turnTokens,
  sessionTotal = 0,
  bridgeAlive = true,
  onSwitchModel,
}: {
  meta?: Meta;
  context: ContextInfo;
  usage?: WireUsage;
  balance?: BalanceInfo;
  jobs?: JobView[];
  running: boolean;
  mode: Mode;
  turnStartAt: number;
  turnTokens: number;
  sessionTotal?: number;
  bridgeAlive?: boolean;
  onSwitchModel: (name: string) => void;
}) {
  const { t } = useI18n();
  const now = useTick(running);
  const nowPct = nowRate(usage);
  const avgPct = avgRate(usage);

  const elapsedMs = running && turnStartAt ? Math.max(0, now - turnStartAt) : 0;
  const elapsed = elapsedMs > 0 ? fmtElapsed(elapsedMs) : "";
  const tokLabel = running && turnTokens > 0 ? `↓${fmtTokens(turnTokens)}` : "";

  // Cache rate color
  const cacheColor = nowPct !== null
    ? nowPct >= 80 ? "text-(--color-success)" : nowPct >= 50 ? "text-(--color-warning)" : "text-(--color-error)"
    : "";

  return (
    <div className="flex items-center gap-1.5 h-8 px-3 text-[11px] bg-(--color-bg-soft) border-t border-(--color-border-soft) select-none shrink-0" data-wails-no-drag>
      {/* Connection state light: green=online, yellow=busy, red=offline */}
      <span
        className={`w-1.5 h-1.5 rounded-full ${
          !bridgeAlive ? "bg-(--color-error)" : running ? "bg-(--color-warning) animate-pulse" : "bg-(--color-success)"
        }`}
        title={!bridgeAlive ? "offline" : running ? "busy" : "online"}
      />

      <ModelSwitcher label={meta?.label ?? t("status.connecting")} onPick={onSwitchModel} />

      <span className="text-(--color-border) select-none mx-0.5">·</span>

      {running ? (
        <>
          <span className="text-(--color-fg-dim) tabular-nums">{elapsed}</span>
          {tokLabel && (
            <>
              <span className="text-(--color-border) select-none mx-0.5">·</span>
              <span className="text-(--color-fg-dim) tabular-nums">{tokLabel}</span>
            </>
          )}
          <span className="flex-1" />
        </>
      ) : (
        <>
          {sessionTotal > 0 && (
            <>
              <span className="text-(--color-fg-dim)">累计 {fmtTokens(sessionTotal)}</span>
              <span className="text-(--color-border) select-none mx-0.5">·</span>
            </>
          )}
          <span className="text-(--color-fg-dim) flex items-center gap-1.5">
            {/* Context gauge bar: 4 segments (reserved/used/cached/free) */}
            {context.window > 0 && (
              <span className="inline-flex gap-px h-2 w-16 rounded-sm overflow-hidden align-middle"
                title={`reserved ${fmtTokens(context.used * 2)} / used ${fmtTokens(context.used)} / free ${fmtTokens(context.window - context.used)}`}
              >
                <span className="bg-(--color-info) opacity-40" style={{ width: `${Math.min(100, (context.used * 2 / context.window) * 100)}%` }} />
                <span className="bg-(--color-info)" style={{ width: `${Math.min(100, (context.used / context.window) * 100)}%` }} />
                <span className="bg-(--color-border-soft) flex-1" />
              </span>
            )}
            {fmtTokens(context.used)}/{fmtTokens(context.window)}
          </span>
          <span className="flex-1" />
        </>
      )}

      {nowPct !== null && (
        <>
          <span className={cacheColor}>{t("status.cache", { pct: nowPct })}</span>
          <span className="text-(--color-border) select-none mx-0.5">·</span>
        </>
      )}
      {avgPct !== null && (
        <span className={cacheColor}>{t("status.cacheAvg", { pct: avgPct })}</span>
      )}

      {jobs && jobs.length > 0 && (
        <>
          <span className="text-(--color-border) select-none mx-0.5">·</span>
          <JobsChip jobs={jobs} />
        </>
      )}
      {balance?.available && balance.display && (
        <>
          <span className="text-(--color-border) select-none mx-0.5">·</span>
          <span className="inline-flex items-center gap-1 text-(--color-fg-dim)" title={t("status.balanceTitle")}>
            <Wallet size={11} />
            {balance.display}
          </span>
        </>
      )}

      {mode === "yolo" && (
        <span className="text-[11px] font-bold text-(--color-error) px-1.5 py-px rounded border border-(--color-error/20) bg-(--color-error/10)" title={t("status.yoloTitle")}>
          {t("status.yolo")}
        </span>
      )}
      {mode === "plan" && (
        <span className="text-[11px] font-medium text-(--color-warning) px-1.5 py-px rounded border border-(--color-warning/20) bg-(--color-warning/10)">
          {t("status.plan")}
        </span>
      )}
      {!bridgeAlive && (
        <span className="text-[11px] text-(--color-error) font-medium">
          ⚠ 桥接断开
        </span>
      )}
    </div>
  );
}
