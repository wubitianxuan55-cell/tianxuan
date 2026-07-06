import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { ModelSwitcher } from "./ModelSwitcher";
import type { SectionProps } from "./SettingsShared";

export function AgentSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  const [temp, setTemp] = useState(String(s.agent.temperature));
  const [steps, setSteps] = useState(String(s.agent.maxSteps));
  const [prompt, setPrompt] = useState(s.agent.systemPrompt);
  const dirty = temp !== String(s.agent.temperature) || steps !== String(s.agent.maxSteps) || prompt !== s.agent.systemPrompt;
  const [skillsOpen, setSkillsOpen] = useState(false);

  const subagentLabel = s.subagentModel || t("settings.subagentInherit");
  const subagentModels = s.subagentModels || {};
  const plannerLabel = s.plannerModel || t("settings.plannerNone");

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold">{t("settings.agent")}</div>

      {/* 全局子代理模型 */}
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] w-[80px] shrink-0">{t("settings.subagentModel")}</label>
        <div className="flex-1">
          <ModelSwitcher
            label={subagentLabel}
            allowInherit
            inheritLabel={t("settings.subagentInherit")}
            onPick={(ref: string) => void apply(() => app.SetSubagentModel(ref))}
          />
        </div>
      </div>

      {/* 按技能单独配置（可折叠） */}
      {(s.subagentSkills || []).length > 0 && (
        <div className="mb-2.5">
          <button
            className="flex items-center gap-1 text-fg-dim text-[13px] font-medium hover:text-fg cursor-pointer bg-transparent border-0 p-0"
            onClick={() => setSkillsOpen((v) => !v)}
          >
            {skillsOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            {t("settings.subagentPerSkill") || "按技能单独配置"}
          </button>
          {skillsOpen && (
            <div className="mt-2 space-y-2 pl-4 border-l-2 border-border-soft">
              {s.subagentSkills.map((skill: string) => {
                const skillRef = subagentModels[skill] || "";
                const globalRef = s.subagentModel;
                const inheritText = globalRef ? `继承全局: ${globalRef}` : t("settings.subagentInherit");
                const skillLabel = skillRef || inheritText;
                return (
                  <div key={skill} className="flex items-center gap-3">
                    <label className="text-fg-dim text-[12px] w-[100px] shrink-0 font-mono">{skill}</label>
                    <div className="flex-1">
                      <ModelSwitcher
                        label={skillLabel}
                        allowInherit
                        inheritLabel={inheritText}
                        onPick={(ref: string) => void apply(() => app.SetSubagentModelForSkill(skill, ref))}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] w-[80px] shrink-0">{t("settings.plannerModel")}</label>
        <div className="flex-1">
          <ModelSwitcher
            label={plannerLabel}
            allowInherit
            inheritLabel={t("settings.plannerNone")}
            onPick={(ref: string) => void apply(() => app.SetPlannerModel(ref))}
          />
        </div>
      </div>

      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] w-[80px] shrink-0">{t("settings.temperature")}</label>
        <input className="w-[70px] bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent text-center" value={temp} onChange={(e) => setTemp(e.target.value)} disabled={busy} inputMode="decimal" />
        <span className="text-fg-faint text-[10px]">0.0–1.0</span>
      </div>
      <div className="flex items-center gap-3 mb-2.5">
        <label className="text-fg-dim text-[13px] w-[80px] shrink-0">{t("settings.maxSteps")}</label>
        <input className="w-[70px] bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent text-center" value={steps} onChange={(e) => setSteps(e.target.value)} disabled={busy} inputMode="numeric" />
        <span className="text-fg-faint text-[10px]">{t("settings.unlimited")}</span>
      </div>
      <div className="text-fg-dim text-[12px] font-medium mb-1">{t("settings.systemPrompt")}</div>
      <textarea className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] p-2.5 outline-none resize-y min-h-[120px] focus:border-accent" value={prompt} onChange={(e) => setPrompt(e.target.value)} disabled={busy} spellCheck={false} />
      <div className="flex gap-2 mt-2">
        <button
          className="btn--primary"
          disabled={busy || !dirty}
          onClick={() => void apply(() => app.SetAgentParams(Number(temp) || 0, Number(steps) || 0, prompt))}
        >
          {t("settings.saveAgent")}
        </button>
      </div>
    </section>
  );
}
