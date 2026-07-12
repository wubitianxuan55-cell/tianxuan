import { useState, useEffect, useCallback, useRef } from "react";
import QRCode from "qrcode";
import { app } from "../lib/bridge";
import type { MobileAccessView } from "../lib/types";

export function MobileSection() {
  const [status, setStatus] = useState<MobileAccessView | null>(null);
  const [port, setPort] = useState(8787);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const qrCanvas = useRef<HTMLCanvasElement>(null);

  // Ngrok state
  const [useNgrok, setUseNgrok] = useState(false);
  const [ngrokToken, setNgrokToken] = useState("");
  const [ngrokInstalled, setNgrokInstalled] = useState<boolean | null>(null);

  const qrURL = status?.publicUrl || status?.url || "";

  const refresh = useCallback(async () => {
    try {
      const s = await app.MobileAccessStatus();
      setStatus(s);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);
  useEffect(() => { app.CheckNgrok().then(setNgrokInstalled); }, []);

  // QR code
  useEffect(() => {
    if (qrURL && qrCanvas.current) {
      QRCode.toCanvas(qrCanvas.current, qrURL, {
        width: 220, margin: 2,
        color: { dark: "#ededef", light: "#0a0a0f" },
      });
    }
  }, [qrURL]);

  const start = async () => {
    setBusy(true); setErr(null);
    try {
      setStatus(await app.StartMobileAccess(port, useNgrok ? ngrokToken.trim() : ""));
    } catch (e: any) { setErr(e?.message || String(e)); }
    finally { setBusy(false); }
  };

  const stop = async () => {
    setBusy(true);
    try { await app.StopMobileAccess(); setStatus(null); } catch (e: any) { setErr(e?.message || String(e)); }
    finally { setBusy(false); }
  };

  const copyURL = () => {
    const u = qrURL;
    if (u) { navigator.clipboard.writeText(u); setCopied(true); setTimeout(() => setCopied(false), 2000); }
  };

  const running = status?.running === true;

  return (
    <div className="space-y-5">
      <h2 className="text-[13px] font-semibold text-fg tracking-wide">移动端远程操控</h2>
      <p className="text-[12.5px] text-fg-dim leading-relaxed">
        启动后手机扫码即可远程对话。开启外网模式可在任何地方访问。
      </p>

      {err && (
        <div className="px-3 py-2 rounded-md bg-danger/10 border border-danger/20 text-danger text-[12px]">{err}</div>
      )}

      {!running ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <label className="text-[12px] text-fg-dim shrink-0">端口</label>
            <input type="number" value={port} onChange={(e) => setPort(Number(e.target.value) || 8787)}
              className="w-24 px-2 py-1.5 rounded-md bg-bg-soft border border-border-soft text-fg text-[13px] outline-none focus:border-accent"
              min={1024} max={65535} />
          </div>

          {/* Ngrok toggle */}
          <div className="bg-bg-soft rounded-lg border border-border-soft p-3 space-y-3">
            <label className="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" checked={useNgrok} onChange={(e) => setUseNgrok(e.target.checked)}
                className="w-4 h-4 rounded accent-accent" />
              <span className="text-[13px] text-fg font-medium">🌐 外网模式 (ngrok)</span>
            </label>

            {useNgrok && (
              <>
                {ngrokInstalled === false && (
                  <div className="text-[12px] text-warn bg-warn/5 border border-warn/20 rounded-md px-3 py-2">
                    ngrok 未安装。
                    <a href="https://ngrok.com/download" target="_blank" className="text-accent underline ml-1">下载 ngrok</a>
                    {" "}并加入 PATH，然后
                    <a href="https://dashboard.ngrok.com/get-started/your-authtoken" target="_blank" className="text-accent underline ml-1">获取 token</a>
                  </div>
                )}
                <input
                  type="text" value={ngrokToken} onChange={(e) => setNgrokToken(e.target.value)}
                  placeholder="粘贴 ngrok authtoken..."
                  className="w-full px-3 py-2 rounded-md bg-bg border border-border-soft text-fg text-[13px] outline-none focus:border-accent placeholder:text-fg-muted"
                />
              </>
            )}
          </div>

          <button onClick={start} disabled={busy || (useNgrok && !ngrokToken.trim())}
            className="w-full py-2.5 rounded-lg bg-accent text-white text-[13px] font-medium hover:opacity-90 disabled:opacity-40 transition-opacity">
            {busy ? "启动中…" : useNgrok ? "🚀 启动外网接入" : "🚀 启动局域网接入"}
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          {/* Status */}
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-success animate-pulse" />
            <span className="text-[13px] text-success font-medium">
              运行中 · 端口 {status.port}
              {status.usingNgrok && !status.ngrokReady && " · 隧道连接中…"}
            </span>
          </div>

          {/* QR code */}
          <div className="flex flex-col items-center bg-bg-soft rounded-xl border border-border-soft p-5">
            <p className="text-[12px] text-fg-dim mb-3">
              {status.usingNgrok && status.ngrokReady ? "🌐 手机扫描二维码（外网可用）" : "📱 手机扫描二维码（局域网）"}
            </p>
            <canvas ref={qrCanvas} className="rounded-lg" />
            {status.usingNgrok && !status.ngrokReady && (
              <p className="text-[12px] text-warn mt-3 animate-pulse">正在连接 ngrok 隧道…</p>
            )}
            <p className="text-[11px] text-fg-dim mt-3 text-center leading-relaxed">
              用手机相机扫一扫<br />自动打开控制面板
            </p>
          </div>

          {/* URL details */}
          <details className="bg-bg-soft rounded-lg border border-border-soft">
            <summary className="px-3 py-2 text-[12px] text-fg-dim cursor-pointer select-none hover:text-fg">手动复制链接</summary>
            <div className="px-3 pb-3 space-y-2">
              <div className="flex items-center gap-2">
                <code className="flex-1 text-[11px] text-fg break-all bg-bg rounded px-2 py-1.5">{qrURL}</code>
                <button onClick={copyURL}
                  className="shrink-0 px-2.5 py-1 rounded-md bg-accent/15 text-accent text-[11px] font-medium hover:bg-accent/25">
                  {copied ? "已复制" : "复制"}
                </button>
              </div>
              {status.publicUrl && (
                <div className="text-[11px] text-fg-dim">
                  <span className="text-success">●</span> 外网可访问
                </div>
              )}
            </div>
          </details>

          <button onClick={stop} disabled={busy}
            className="w-full py-2.5 rounded-lg bg-danger/10 text-danger text-[13px] font-medium border border-danger/20 hover:bg-danger/20 disabled:opacity-40 transition-colors">
            ⏹ 停止接入
          </button>
        </div>
      )}
    </div>
  );
}
