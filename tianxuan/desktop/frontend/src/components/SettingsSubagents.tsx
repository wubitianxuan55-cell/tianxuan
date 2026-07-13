import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";
import type { SectionProps } from "./SettingsShared";

export function SettingsSubagents({ s }: SectionProps) {
  const skills = s.subagentSkills || [];
  const subagentModels = s.subagentModels || {};

  return (
    <SettingsPageShell title="子代理" desc="管理 task / explore / review 等子代理的模型分配和运行时参数。">
      <SettingsSection title="全局设置">
        <div className="bg-bg-soft border border-border-soft rounded-lg px-4 py-3 mb-2">
          <div className="flex items-center justify-between">
            <div>
              <span className="text-[12px] text-fg-faint">默认模型</span>
              <div className="text-[13px] font-medium text-fg mt-0.5">{s.subagentModel || "（继承主模型）"}</div>
            </div>
            <div>
              <span className="text-[12px] text-fg-faint">推理力度</span>
              <div className="text-[13px] font-medium text-fg mt-0.5">{s.agent.subagentEffort || s.agent.effort || "默认"}</div>
            </div>
            <div>
              <span className="text-[12px] text-fg-faint">递归深度</span>
              <div className="text-[13px] font-medium text-fg mt-0.5">{s.agent.maxSubagentDepth || "不限"}</div>
            </div>
          </div>
        </div>
      </SettingsSection>

      <SettingsSection title={`按技能覆盖（${skills.length} 个技能）`}>
        {skills.length === 0 ? (
          <p className="text-[12px] text-fg-faint">无内置子代理技能。</p>
        ) : (
          <div className="flex flex-col gap-2">
            {skills.map((skill: string) => {
              const ref = subagentModels[skill] || "";
              return (
                <div key={skill} className="bg-bg-soft border border-border-soft rounded-lg px-4 py-3 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="text-[13px] font-mono font-medium text-fg">{skill}</span>
                    {!ref && <span className="text-[10px] px-1.5 py-0.5 rounded bg-bg-elev-2 text-fg-faint">继承全局</span>}
                  </div>
                  <div className="text-[12px] text-fg">
                    {ref || "同默认模型"}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </SettingsSection>
    </SettingsPageShell>
  );
}
