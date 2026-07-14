import { useEffect, useMemo, useState } from "react";
import { Search, Zap, X } from "lucide-react";
import { app } from "../lib/bridge";
import type { SkillView } from "../lib/types";
import { SettingsPageShell, SettingsSection, SettingsField } from "./SettingsPageShell";

const SCOPE_LABEL: Record<string, { label: string; cls: string }> = {
  builtin: { label: "内置", cls: "text-blue-400 bg-blue-400/10" },
  project: { label: "项目", cls: "text-green-400 bg-green-400/10" },
  custom:  { label: "自定义", cls: "text-amber-400 bg-amber-400/10" },
  global:  { label: "全局", cls: "text-purple-400 bg-purple-400/10" },
};

const RUN_LABEL: Record<string, string> = { inline: "inline", subagent: "subagent" };

export function SettingsSkills() {
  const [skills, setSkills] = useState<SkillView[]>([]);
  const [loading, setLoading] = useState(true);
  const [paths, setPaths] = useState<string[]>([]);
  const [pathsErr, setPathsErr] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  useEffect(() => {
    app.Capabilities().then((c) => setSkills(c.skills)).finally(() => setLoading(false));
    app.SkillsSettingsAdvanced().then((v) => setPaths(v.paths || [])).catch(() => {});
  }, []);

  const savePaths = async (next: string[]) => {
    setPaths(next);
    try { await app.SaveSkillsSettings({ paths: next }); setPathsErr(null); } catch (e: any) { setPathsErr(String(e)); }
  };

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return skills;
    return skills.filter((s) => `${s.name} ${s.description}`.toLowerCase().includes(q));
  }, [skills, query]);

  const pathsText = paths.join("\n");

  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><Zap size={15} />技能</span>} desc="可发现的智能体技能，按作用域分组。自定义路径在 config.toml [skills] 中配置。">
      {loading ? <p className="text-[13px] text-fg-faint">加载中…</p> :
       skills.length === 0 ? <p className="text-[13px] text-fg-faint">未发现技能。</p> :
       <>
         <div className="mb-3">
           <div className="mem-search">
             <Search size={14} className="mem-search__icon" />
             <input
               className="mem-filter"
               placeholder="搜索技能…"
               value={query}
               onChange={(e) => setQuery(e.target.value)}
               spellCheck={false}
             />
             {query && <button className="mem-search__clear" onClick={() => setQuery("")}><X size={12} /></button>}
           </div>
         </div>
         <SettingsSection title={<span>{filtered.length} 个技能</span>}>
           <div className="flex flex-col gap-1.5">
             {filtered.map((s) => {
               const scope = SCOPE_LABEL[s.scope] || { label: s.scope, cls: "text-fg-faint bg-bg-elev" };
               return (
                 <div key={s.name} className="flex items-center gap-3 bg-bg-soft border border-border-soft rounded-lg px-3 py-2.5 hover:border-fg-faint/30 transition-colors">
                   <div className="flex-1 min-w-0">
                     <div className="flex items-center gap-2">
                       <span className="text-[13px] font-semibold text-fg font-mono">{s.name}</span>
                       <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${scope.cls}`}>{scope.label}</span>
                     </div>
                     <div className="text-[11px] text-fg-faint mt-0.5 line-clamp-2">{s.description}</div>
                   </div>
                   <span className="text-[10px] text-fg-faint shrink-0 bg-bg-elev px-1.5 py-0.5 rounded">{RUN_LABEL[s.runAs] || s.runAs}</span>
                 </div>
               );
             })}
           </div>
         </SettingsSection>
       </>
      }

      <SettingsSection title="自定义路径">
        {pathsErr && <div className="bg-red-900/20 border border-red-500/30 rounded-md text-red-300 text-[12px] px-3 py-2 mb-2">{pathsErr}</div>}
        <SettingsField label="路径" hint="每行一个目录。支持 ~ 和 ${VAR} 展开。">
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
