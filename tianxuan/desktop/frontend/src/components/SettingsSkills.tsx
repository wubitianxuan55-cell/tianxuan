import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import type { SkillView } from "../lib/types";
import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";

const SCOPE_LABEL: Record<string, string> = { builtin: "内置", project: "项目", custom: "自定义", global: "全局" };
const RUN_LABEL: Record<string, string> = { inline: "内联", subagent: "子代理" };

export function SettingsSkills() {
  const [skills, setSkills] = useState<SkillView[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    app.Capabilities().then((c) => setSkills(c.skills)).finally(() => setLoading(false));
  }, []);

  return (
    <SettingsPageShell title="技能" desc="可发现的 Agent 技能列表，按作用域分组。">
      {loading ? <p className="text-[13px] text-fg-faint">加载中…</p> :
       skills.length === 0 ? <p className="text-[13px] text-fg-faint">未发现技能。在项目或全局 .reasonix/skills/ 目录中添加 SKILL.md 文件。</p> :
       <SettingsSection title={`${skills.length} 个技能`}>
         <div className="flex flex-col gap-1.5">
           {skills.map((s) => (
             <div key={s.name} className="flex items-center gap-3 bg-bg-soft border border-border-soft rounded-lg px-3 py-2.5">
               <div className="flex-1 min-w-0">
                 <div className="flex items-center gap-2">
                   <span className="text-[13px] font-medium text-fg">{s.name}</span>
                   <span className="text-[10px] px-1.5 py-0.5 rounded bg-accent-soft text-accent font-medium">{SCOPE_LABEL[s.scope] || s.scope}</span>
                 </div>
                 <div className="text-[11px] text-fg-faint mt-0.5">{s.description}</div>
               </div>
               <span className="text-[10px] text-fg-faint shrink-0">{RUN_LABEL[s.runAs] || s.runAs}</span>
             </div>
           ))}
         </div>
       </SettingsSection>
      }
    </SettingsPageShell>
  );
}
