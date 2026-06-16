import { useEffect, useState } from "react";
import { Cpu, Wallet } from "lucide-react";
import { ModelSwitcher } from "./ModelSwitcher";
import { useI18n } from "../lib/i18n";
import type { BalanceInfo, ContextInfo, JobView, Meta, Mode, WireUsage } from "../lib/types";

// JobsChip is the status-bar background-jobs indicator: a count that opens an
// upward popover listing the running jobs (id · label · status), mirroring the
// ModelSwitcher's click-to-open pattern. It renders nothing when there are no
// jobs, so the caller guards on jobs.length first.
function JobsChip({ jobs }: { jobs: JobView[] }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return (
    <div className="statusbar__jobswrap">
      <button className="statusbar__jobs" onClick={() => setOpen((v) => !v)} title={t("status.jobsTitle")}>
        <Cpu size={11} />
        {t("status.jobs", { n: jobs.length })}
      </button>
      {open && (
        <>
          <div className="modelsw__backdrop" onClick={() => setOpen(false)} />
          <div className="modelsw__menu jobsmenu" role="listbox">
            <div className="jobsmenu__head">{t("status.jobsTitle")}</div>
            {jobs.map((j) => (
              <div className="jobsmenu__item" key={j.id} role="option">
                <span className="jobsmenu__id">{j.id}</span>
                <span className="jobsmenu__label">{j.label || j.kind}</span>
                <span className="jobsmenu__status">{j.status}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// nowRate is the SINGLE-TURN prompt cache-hit % (latest turn) — the higher,
// steeper number on a non-compacting DeepSeek session. null when nothing yet.
function nowRate(u?: WireUsage): number | null {
  if (!u) return null;
  let denom = u.cacheHitTokens + u.cacheMissTokens;
  if (denom === 0) denom = u.promptTokens;
  if (denom <= 0) return null;
  return Math.round((u.cacheHitTokens / denom) * 100);
}

// avgRate is the SESSION-AGGREGATE cache-hit % — Σhit/Σ(hit+miss) across every
// turn — the steadier, cost-oriented number that matches the legacy dashboard.
// On a non-compacting DeepSeek session it trails nowRate (early cold-start turns
// drag the average down); it overtakes only when compaction craters single turns.
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

// useTick re-renders once a second while `on`, so the elapsed clock advances.
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

  // While a turn runs, show elapsed time and token count inline.
  const elapsedMs = running && turnStartAt ? Math.max(0, now - turnStartAt) : 0;
  const elapsed = elapsedMs > 0 ? fmtElapsed(elapsedMs) : "";
  const tokLabel = running && turnTokens > 0 ? `↓${fmtTokens(turnTokens)}` : "";

  return (
    <div className="statusbar">
      <span className={`statusbar__dot ${running ? "statusbar__dot--busy" : ""}`} />
      <ModelSwitcher label={meta?.label ?? t("status.connecting")} onPick={onSwitchModel} />
      {running ? (
        <>
          <span className="statusbar__sep">·</span>
          <span className="statusbar__activity">{elapsed}</span>
          {tokLabel && (<><span className="statusbar__sep">·</span><span className="statusbar__tokens">{tokLabel}</span></>)}
          <span className="statusbar__spacer" />
        </>
      ) : (
        <>
          {sessionTotal > 0 && (
            <>
              <span className="statusbar__sep">·</span>
              <span className="statusbar__tokens">累计 {fmtTokens(sessionTotal)}</span>
            </>
          )}
          <>
            <span className="statusbar__sep">·</span>
            <span className="statusbar__ctx">{fmtTokens(context.used)}/{fmtTokens(context.window)}</span>
          </>
        </>
      )}
      {nowPct !== null && (
        <>
          <span className="statusbar__sep">·</span>
          <span className="statusbar__cache">{t("status.cache", { pct: nowPct })}</span>
        </>
      )}
      {avgPct !== null && (
        <>
          <span className="statusbar__sep">·</span>
          <span className="statusbar__cache">{t("status.cacheAvg", { pct: avgPct })}</span>
        </>
      )}
      {jobs && jobs.length > 0 && (
        <>
          <span className="statusbar__sep">·</span>
          <JobsChip jobs={jobs} />
        </>
      )}
      {balance?.available && balance.display && (
        <>
          <span className="statusbar__sep">·</span>
          <span className="statusbar__balance" title={t("status.balanceTitle")}>
            <Wallet size={11} />
            {balance.display}
          </span>
        </>
      )}
      <span className="statusbar__spacer" />
      {mode === "yolo" && (
        <span className="statusbar__yolo" title={t("status.yoloTitle")}>
          {t("status.yolo")}
        </span>
      )}
      {mode === "plan" && <span className="statusbar__plan">{t("status.plan")}</span>}
      {!bridgeAlive && (
        <span className="statusbar__bridge" title="与后端桥接断开——请重启应用">
          ⚠ 桥接断开
        </span>
      )}
    </div>
  );
}
