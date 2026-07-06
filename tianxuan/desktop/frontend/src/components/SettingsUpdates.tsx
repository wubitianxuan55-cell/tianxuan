import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { useUpdater } from "../lib/useUpdater";

const MB = 1024 * 1024;
const mb = (n: number) => (n / MB).toFixed(1);

// UpdatesSection is the manual side of the auto-updater: it shows the running
// version and a Check button, then the same state machine the top banner uses
// (useUpdater) — available → install/download, with progress and errors inline.
export function UpdatesSection({ configPath }: { configPath: string }) {
  const t = useT();
  const { status, check, apply } = useUpdater();
  const [version, setVersion] = useState("");
  useEffect(() => {
    app.Version().then(setVersion).catch(() => {});
  }, []);

  const busy =
    status.kind === "checking" || status.kind === "downloading" || status.kind === "verifying" || status.kind === "applying";

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold">{t("updater.title")}</div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] shrink-0">{t("updater.currentVersion", { v: version || "…" })}</label>
        <span className="flex-1" />
        <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" disabled={busy} onClick={() => void check()}>
          {status.kind === "checking" ? t("updater.checking") : t("updater.checkButton")}
        </button>
      </div>
      {status.kind === "upToDate" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.upToDate")}</div>}
      {status.kind === "available" && (
        <>
          <div className="flex items-center gap-3 mb-2.5">
            <span className="text-fg-dim text-[13px] shrink-0">{t("updater.available", { v: status.info.latest })}</span>
            <span className="flex-1" />
            <button className="btn--primary" onClick={() => apply(status.info)}>
              {status.info.canSelfUpdate ? t("updater.installNow") : t("updater.goToDownload")}
            </button>
          </div>
          {!status.info.canSelfUpdate && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.macHint")}</div>}
        </>
      )}
      {status.kind === "downloading" && (
        <div className="text-fg-faint text-[10px] mt-1 px-1">
          {t("updater.downloading", {
            done: mb(status.received),
            total: mb(status.total),
            pct: status.total > 0 ? Math.round((status.received / status.total) * 100) : 0,
          })}
        </div>
      )}
      {status.kind === "verifying" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.verifying")}</div>}
      {status.kind === "applying" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.applying")}</div>}
      {status.kind === "done" && <div className="text-fg-faint text-[10px] mt-1 px-1">{t("updater.done")}</div>}
      {status.kind === "error" && <div className="shrink-0 px-4 py-2 text-[12.5px] bg-del-bg text-err border-b border-border-soft">{t("updater.failed", { msg: status.message })}</div>}
      {configPath && (
        <div className="text-fg-faint text-[10px] mt-1 px-1 font-mono truncate" title={configPath}>
          {t("settings.config", { path: configPath })}
        </div>
      )}
    </section>
  );
}
