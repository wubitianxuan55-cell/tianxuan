import { useEffect, useState } from "react";
import { useT } from "../lib/i18n";
import { useUpdater } from "../lib/useUpdater";

const MB = 1024 * 1024;
const mb = (n: number) => (n / MB).toFixed(1);

// UpdateBanner checks for an update once on mount and, when one is available, shows
// a dismissible top banner that drives the download → verify → install flow (or, on
// macOS, links out to the download page). It renders nothing while idle, checking,
// or already current — a quiet auto-check that only surfaces when actionable. A
// failed check is silent here too (network blips shouldn't nag); the Settings panel
// is where a manual check shows errors.
export function UpdateBanner() {
  const t = useT();
  const { status, check, apply } = useUpdater();
  const [dismissed, setDismissed] = useState<string | null>(null);

  useEffect(() => {
    void check();
  }, [check]);

  switch (status.kind) {
    case "available": {
      const info = status.info;
      if (info.latest === dismissed) return null;
      return (
        <div className="shrink-0 px-4 py-2 text-[12.5px] flex items-center gap-2.5 bg-accent-soft text-fg border-b border-border-soft">
          <span className="font-medium">{t("updater.available", { v: info.latest })}</span>
          {!info.canSelfUpdate && <span className="text-fg-dim text-[11.5px]">{t("updater.macHint")}</span>}
          <span className="flex-1" />
          <button className="px-2.5 py-1 text-xs" onClick={() => apply(info)}>
            {info.canSelfUpdate ? t("updater.installNow") : t("updater.goToDownload")}
          </button>
          <button className="px-2.5 py-1 text-xs" onClick={() => setDismissed(info.latest)}>
            {t("updater.dismiss")}
          </button>
        </div>
      );
    }
    case "downloading": {
      const pct = status.total > 0 ? Math.round((status.received / status.total) * 100) : 0;
      return (
        <div className="shrink-0 px-4 py-2 text-[12.5px] flex items-center gap-2.5 bg-accent-soft text-fg border-b border-border-soft">
          <span className="font-medium">
            {t("updater.downloading", { done: mb(status.received), total: mb(status.total), pct })}
          </span>
          <span className="flex-1" />
          <progress className="w-[180px] h-2 accent-accent" value={status.received} max={status.total || undefined} />
        </div>
      );
    }
    case "verifying":
      return <div className="shrink-0 px-4 py-2 text-[12.5px] flex items-center gap-2.5 bg-accent-soft text-fg border-b border-border-soft">{t("updater.verifying")}</div>;
    case "applying":
      return <div className="shrink-0 px-4 py-2 text-[12.5px] flex items-center gap-2.5 bg-accent-soft text-fg border-b border-border-soft">{t("updater.applying")}</div>;
    case "done":
      return <div className="shrink-0 px-4 py-2 text-[12.5px] flex items-center gap-2.5 bg-accent-soft text-fg border-b border-border-soft">{t("updater.done")}</div>;
    case "error":
      // 自动更新检查失败时静默（网络波动不打扰用户），手动检查在 Settings 面板中显示错误。
      return null;
    default:
      // idle | checking | upToDate — nothing to show.
      return null;
  }
}
