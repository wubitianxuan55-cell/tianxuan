import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import type { SkillView } from "../lib/types";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";

const SCOPE_LABEL: Record<string, string> = { builtin: "builtin", project: "project", custom: "custom", global: "global" };
const RUN_LABEL: Record<string, string> = { inline: "inline", subagent: "subagent" };

interface SkillsSettingsView { paths: string[] }

export function SettingsSkills() {
  const [skills, setSkills] = useState<SkillView[]>([]);
  const [loading, setLoading] = useState(true);
  const [paths, setPaths] = useState<string[]>([]);
  const [pathsErr, setPathsErr] = useState<string | null>(null);

  useEffect(() => {
    app.Capabilities().then((c) => setSkills(c.skills)).finally(() => setLoading(false));
    (app as any).SkillsSettingsAdvanced().then((v: SkillsSettingsView) => setPaths(v.paths || [])).catch(() => {});
  }, []);

  const savePaths = async (next: string[]) => {
    setPaths(next);
    try { await (app as any).SaveSkillsSettings({ paths: next }); setPathsErr(null); } catch (e: any) { setPathsErr(String(e)); }
  };

  const pathsText = paths.join("\n");

  return (
    <SettingsPageShell title="Skills" desc="Discoverable agent skills grouped by scope. Custom paths in config.toml [skills].">
      {loading ? <p className="text-[13px] text-fg-faint">Loading...</p> :
       skills.length === 0 ? <p className="text-[13px] text-fg-faint">No skills found.</p> :
       <SettingsSection title={skills.length + " skills"}>
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

      {/* Advanced: custom skill paths */}
      <SettingsSection title="Custom Paths">
        {pathsErr && <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-2">{pathsErr}</div>}
        <SettingsField label="Paths" hint="One directory per line. ~ and ${VAR} expansion supported.">
          <textarea className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none focus:border-accent font-mono resize-y min-h-[80px]"
            value={pathsText}
            onChange={e => savePaths(e.target.value.split("\n").map(s => s.trim()).filter(Boolean))}
            rows={4}
          />
        </SettingsField>
      </SettingsSection>
    </SettingsPageShell>
  );
}
