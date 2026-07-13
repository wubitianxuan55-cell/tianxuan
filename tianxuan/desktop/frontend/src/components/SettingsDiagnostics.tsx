import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";

interface DiagItem { label: string; status: "ok" | "warn" | "err"; detail: string }

export function SettingsDiagnostics() {
  const [items, setItems] = useState<DiagItem[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const run = async () => {
      const diags: DiagItem[] = [];
      try {
        const caps = await app.Capabilities();
        const connected = caps.servers.filter(s => s.status === "connected").length;
        const failed = caps.servers.filter(s => s.status === "failed").length;
        diags.push({ label: "MCP 服务器", status: failed === 0 ? "ok" : "warn", detail: `${connected} 已连接 / ${failed} 异常 / ${caps.servers.length} 总计` });
        
        const skills = caps.skills.length;
        diags.push({ label: "技能发现", status: skills > 0 ? "ok" : "warn", detail: `${skills} 个可发现技能` });
      } catch {
        diags.push({ label: "能力检查", status: "err", detail: "无法获取 Capabilities 数据" });
      }

      try {
        const mem = await app.Memory();
        diags.push({ label: "记忆系统", status: mem.available ? "ok" : "warn", detail: `${mem.facts.length} 条记忆 · ${mem.docs.length} 个文档` });
      } catch {
        diags.push({ label: "记忆系统", status: "warn", detail: "未初始化" });
      }

      try {
        const ver = await app.Version();
        diags.push({ label: "版本", status: "ok", detail: ver });
      } catch {
        diags.push({ label: "版本", status: "warn", detail: "未知" });
      }

      try {
        const ctx = await app.ContextUsage();
        diags.push({ label: "上下文状态", status: "ok", detail: `已用 ${ctx.used} / ${ctx.window} tokens` });
      } catch {
        diags.push({ label: "上下文状态", status: "warn", detail: "无活跃会话" });
      }

      setItems(diags);
      setLoading(false);
    };
    void run();
  }, []);

  const statusIcon = (s: string) => {
    if (s === "ok") return <span className="w-2.5 h-2.5 rounded-full bg-[#22C55E] inline-block shrink-0" />;
    if (s === "warn") return <span className="w-2.5 h-2.5 rounded-full bg-[#F59E0B] inline-block shrink-0" />;
    return <span className="w-2.5 h-2.5 rounded-full bg-[#EF4444] inline-block shrink-0" />;
  };

  return (
    <SettingsPageShell title="诊断" desc="检查 tianxuan 各子系统运行状态。">
      {loading ? <p className="text-[13px] text-fg-faint">正在运行诊断…</p> : (
        <SettingsSection title={`${items.length} 项检查`}>
          <div className="flex flex-col gap-1.5">
            {items.map((d, i) => (
              <div key={i} className="flex items-center gap-3 bg-bg-soft border border-border-soft rounded-lg px-4 py-3">
                {statusIcon(d.status)}
                <div className="flex-1 min-w-0">
                  <div className="text-[13px] font-medium text-fg">{d.label}</div>
                  <div className="text-[11px] text-fg-faint">{d.detail}</div>
                </div>
                <span className="text-[10px] text-fg-faint shrink-0">{d.status === "ok" ? "✓" : d.status === "warn" ? "⚠" : "✗"}</span>
              </div>
            ))}
          </div>
        </SettingsSection>
      )}
    </SettingsPageShell>
  );
}
