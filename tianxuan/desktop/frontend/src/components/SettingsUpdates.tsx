import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { CloudUpload } from "lucide-react";
import { useUpdater } from "../lib/useUpdater";
import { SettingsPageShell } from "./SettingsPageShell";

const MB = 1024 * 1024;
const mb = (n: number) => (n / MB).toFixed(1);

export function UpdatesSection({ configPath }: { configPath: string }) {
  const t = useT();
  const { status, check, apply } = useUpdater();
  const [version, setVersion] = useState("");
  useEffect(() => { app.Version().then(setVersion).catch(() => {}); }, []);

  const busy = status.kind === "checking" || status.kind === "downloading" || status.kind === "verifying" || status.kind === "applying";

  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><CloudUpload size={15} />更新</span>} desc="检查 tianxuan 桌面端新版本并在线更新。">
      {/* 当前版本 */}
      <div className="bg-bg-soft border border-border-soft rounded-lg px-4 py-3 mb-4">
        <div className="flex items-center justify-between">
          <div>
            <div className="text-[12px] text-fg-faint">{t("updater.currentVersion", { v: version || "…" })}</div>
            {status.kind === "upToDate" && <div className="text-[11px] text-[#22C55E] mt-0.5">{t("updater.upToDate")}</div>}
          </div>
          <button className="px-3 py-1.5 text-[12px] rounded-md border border-border-soft bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
            disabled={busy}
            onClick={() => void check()}>
            {status.kind === "checking" ? t("updater.checking") : t("updater.checkButton")}
          </button>
        </div>

        {status.kind === "available" && (
          <div className="mt-3 pt-3 border-t border-border-soft">
            <div className="flex items-center justify-between">
              <span className="text-[13px] text-fg font-medium">{t("updater.available", { v: status.info.latest })}</span>
              <button className="px-3 py-1.5 text-[12px] rounded-md bg-accent text-white border-0 cursor-pointer hover:opacity-90"
                onClick={() => apply(status.info)}>
                {status.info.canSelfUpdate ? t("updater.installNow") : t("updater.goToDownload")}
              </button>
            </div>
            {status.info.notes && (
              <div className="mt-2 text-[11px] text-fg-faint whitespace-pre-wrap">{status.info.notes}</div>
            )}
            {!status.info.canSelfUpdate && <div className="text-[11px] text-fg-faint mt-1">{t("updater.macHint")}</div>}
          </div>
        )}

        {(status.kind === "downloading" || status.kind === "verifying" || status.kind === "applying") && (
          <div className="mt-3 pt-3 border-t border-border-soft">
            {status.kind === "downloading" && (
              <div className="text-[12px] text-fg-faint">
                {t("updater.downloading", { done: mb(status.received), total: mb(status.total), pct: status.total > 0 ? Math.round((status.received / status.total) * 100) : 0 })}
              </div>
            )}
            {status.kind === "verifying" && <div className="text-[12px] text-fg-faint">{t("updater.verifying")}</div>}
            {status.kind === "applying" && <div className="text-[12px] text-fg-faint">{t("updater.applying")}</div>}
          </div>
        )}

        {status.kind === "error" && (
          <div className="mt-2 px-3 py-2 text-[12px] bg-del-bg text-err rounded-md">{t("updater.failed", { msg: status.message })}</div>
        )}
      </div>

      {/* 配置文件路径 */}
      <div className="bg-bg-soft border border-border-soft rounded-lg px-4 py-3">
        <div className="text-[12px] text-fg-faint mb-1">配置文件</div>
        <div className="text-[12px] text-fg font-mono break-all">{configPath || "~/.config/tianxuan/config.toml"}</div>
      </div>
    </SettingsPageShell>
  );
}
