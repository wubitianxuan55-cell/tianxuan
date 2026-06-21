import { useEffect, useState } from "react";
import { Blocks } from "lucide-react";
import { app } from "../lib/bridge";
import type { CapabilitiesView } from "../lib/types";

/** SkillsPanel — 右边栏"技能"标签页，显示全部已发现技能及使用次数 */
export function SkillsPanel({ counts }: { counts: Record<string, number> }) {
  const [skills, setSkills] = useState<CapabilitiesView["skills"]>([]);
  useEffect(() => {
    app.Capabilities().then((v) => setSkills(v.skills)).catch(() => setSkills([]));
  }, []);

  return (
    <div className="flex flex-col overflow-y-auto text-xs">
      <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border-soft text-fg-dim font-semibold text-[11px]">
        <Blocks size={12} />
        <span>技能</span>
      </div>
      <div className="py-1">
        {skills.length === 0 ? (
          <div className="empty-state">加载中…</div>
        ) : (
          <div className="px-1.5 py-1">
            <div className="flex flex-col gap-0.5">
              {skills.map((sk) => {
                const active = (counts[sk.name] ?? 0) > 0;
                return (
                <div
                  key={sk.name}
                  className={`flex items-center gap-1 px-1.5 py-1 rounded-md border border-border-soft bg-bg ${active ? "border-accent-soft bg-sidebar-active" : ""}`}
                  title={sk.description}
                >
                  <span className={`w-1 h-1 rounded-full shrink-0 ${active ? "bg-accent" : "bg-border-soft"}`} />
                  <span className={`font-mono text-[10.5px] flex-1 overflow-hidden text-ellipsis whitespace-nowrap ${active ? "text-accent font-semibold" : "text-fg-dim"}`}>{sk.name}</span>
                  <span className={`font-mono text-[11px] font-semibold ${active ? "text-accent" : "text-fg-faint"}`}>{counts[sk.name] ?? 0}</span>
                </div>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
