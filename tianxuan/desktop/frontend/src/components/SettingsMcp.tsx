import { useEffect, useState } from "react";
import { Wrench, RefreshCw, Server } from "lucide-react";
import { app } from "../lib/bridge";
import type { ServerView } from "../lib/types";
import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";

export function SettingsMcp() {
  const [servers, setServers] = useState<ServerView[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");

  const reload = async () => {
    setLoading(true);
    try {
      const c = await app.Capabilities();
      setServers(c.servers);
      setErr("");
    } catch (e) { setErr(String(e)); }
    finally { setLoading(false); }
  };
  useEffect(() => { void reload(); }, []);

  const status = (s: string) => {
    switch (s) {
      case "connected": return { cls: "bg-emerald-400 animate-[pulse_2s_ease-in-out_infinite]", label: "已连接" };
      case "failed":    return { cls: "bg-red-400", label: "异常" };
      default:          return { cls: "bg-slate-500", label: "已停用" };
    }
  };

  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><Wrench size={15} />MCP 服务器</span>} desc="管理 Model Context Protocol 服务器连接与工具发现。">
      <div className="flex items-center justify-between mb-3">
        <span />
        <button className="btn btn--ghost btn--tiny" onClick={() => void reload()} disabled={loading} type="button">
          <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
        </button>
      </div>

      {loading ? <p className="text-[13px] text-fg-faint">加载中…</p> :
       err ? <p className="text-[13px] text-err">{err}</p> :
       servers.length === 0 ? (
         <div className="flex flex-col items-center py-8 text-fg-faint">
           <Server size={32} className="mb-3 text-fg-faint/30" />
           <p className="text-[13px] text-center max-w-[280px]">未配置 MCP 服务器。用 `tianxuan setup` 或编辑 config.toml 的 [[plugins]] 段添加。</p>
         </div>
       ) : (
         <SettingsSection title={<span>{servers.length} 个服务器</span>}>
           <div className="flex flex-col gap-2">
             {servers.map((s) => {
               const st = status(s.status);
               return (
                 <div key={s.name} className="flex items-center gap-3 bg-bg-soft border border-border-soft rounded-lg px-3 py-2.5 hover:border-fg-faint/30 transition-colors">
                   <span className={`shrink-0 w-2.5 h-2.5 rounded-full ${st.cls}`} title={st.label} />
                   <div className="flex-1 min-w-0">
                     <div className="flex items-center gap-2">
                       <span className="text-[13px] font-semibold text-fg font-mono">{s.name}</span>
                       <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${s.status === "connected" ? "text-emerald-400 bg-emerald-400/10" : s.status === "failed" ? "text-red-400 bg-red-400/10" : "text-fg-faint bg-bg-elev"}`}>{st.label}</span>
                     </div>
                     <div className="text-[11px] text-fg-faint mt-0.5">{s.transport} · {s.tools} 个工具</div>
                   </div>
                   <button
                     className="px-2.5 py-1 text-[11px] rounded border border-border-soft bg-transparent cursor-pointer transition-colors hover:bg-bg-elev disabled:opacity-30 disabled:cursor-default"
                     style={{color: s.status === "failed" ? "var(--accent)" : "var(--fg-faint)"}}
                     disabled={s.status !== "failed"}
                     onClick={async () => { await app.RetryMCPServer(s.name); void reload(); }}
                   >
                     <RefreshCw size={12} className="mr-1 inline-block align-[-1px]" />重试
                   </button>
                 </div>
               );
             })}
           </div>
         </SettingsSection>
       )}
    </SettingsPageShell>
  );
}
