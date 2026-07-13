import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import type { ServerView } from "../lib/types";
import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";

export function SettingsMcp() {
  const [servers, setServers] = useState<ServerView[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");

  const reload = async () => {
    try {
      const c = await app.Capabilities();
      setServers(c.servers);
    } catch (e) { setErr(String(e)); }
    finally { setLoading(false); }
  };
  useEffect(() => { void reload(); }, []);

  const statusIcon = (s: string) => {
    if (s === "connected") return <span className="w-2 h-2 rounded-full bg-[#22C55E] inline-block" />;
    if (s === "failed") return <span className="w-2 h-2 rounded-full bg-[#EF4444] inline-block" />;
    return <span className="w-2 h-2 rounded-full bg-[#6B7280] inline-block" />;
  };
  const statusLabel = (s: string) => ({ connected: "已连接", failed: "异常", disabled: "已停用" }[s] || s);

  return (
    <SettingsPageShell title="MCP 服务器" desc="管理 Model Context Protocol 服务器连接与工具发现。">
      {loading ? <p className="text-[13px] text-fg-faint">加载中…</p> :
       err ? <p className="text-[13px] text-err">{err}</p> :
       servers.length === 0 ? <p className="text-[13px] text-fg-faint">未配置 MCP 服务器。用 `tianxuan setup` 或编辑 config.toml 的 [[plugins]] 段添加。</p> :
       <SettingsSection title={`${servers.length} 个服务器`}>
         <div className="flex flex-col gap-2">
           {servers.map((s) => (
             <div key={s.name} className="flex items-center gap-3 bg-bg-soft border border-border-soft rounded-lg px-3 py-2.5">
               <span className="shrink-0">{statusIcon(s.status)}</span>
               <div className="flex-1 min-w-0">
                 <div className="text-[13px] font-medium text-fg">{s.name}</div>
                 <div className="text-[11px] text-fg-faint">{s.transport} · {s.tools} 工具 · {statusLabel(s.status)}</div>
               </div>
               {s.status === "failed" && (
                 <button className="px-2.5 py-1 text-[11px] rounded border border-border-soft bg-transparent text-accent cursor-pointer hover:bg-accent-soft"
                   onClick={async () => { await app.RetryMCPServer(s.name); void reload(); }}>
                   重试
                 </button>
               )}
             </div>
           ))}
         </div>
       </SettingsSection>
      }
    </SettingsPageShell>
  );
}
