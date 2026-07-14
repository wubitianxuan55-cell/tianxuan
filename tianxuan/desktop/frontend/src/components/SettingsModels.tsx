import { useState } from "react";
import { Cpu, Brain, Bot, ChevronDown, ChevronRight } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { ModelPicker } from "./ModelPicker";
import { StepLimitControl } from "./StepLimitControl";
import { toRef, type SectionProps } from "./SettingsShared";

function ModelCard({ icon, title, desc, children }: { icon: React.ReactNode; title: string; desc: string; children: React.ReactNode }) {
  return (
    <div className="bg-bg-soft border border-border-soft rounded-lg p-3.5 mb-3">
      <div className="flex items-center gap-2 mb-2.5">
        <span className="text-accent shrink-0">{icon}</span>
        <div>
          <div className="text-fg text-[13px] font-semibold leading-tight">{title}</div>
          <div className="text-fg-faint text-[11px] leading-tight">{desc}</div>
        </div>
      </div>
      {children}
    </div>
  );
}

const EFFORT_LEVELS = [
  { key: "", label: "关闭", hint: "直接输出，不显示思考过程" },
  { key: "high", label: "标准", hint: "适度推理，日常任务推荐" },
  { key: "max", label: "深度", hint: "扩展推理链，复杂逻辑任务" },
] as const;

export function EffortSelect({ value, onChange, busy }: { value: string; onChange: (e: string) => void; busy: boolean }) {
  const v = value ?? "";
  return (
    <div className="flex items-center gap-1">
      <span className="text-fg-faint text-[11px] shrink-0 mr-0.5">思考</span>
      {EFFORT_LEVELS.map((l) => (
        <button key={l.key}
          className={`px-2 py-0.5 text-[11px] border rounded transition-colors ${
            v === l.key
              ? "text-accent border-accent bg-accent/15 font-semibold ring-1 ring-accent/30"
              : "text-fg-dim border-border-soft bg-transparent hover:text-fg hover:border-fg-faint"
          }`}
          disabled={busy}
          title={l.hint}
          onClick={() => onChange(l.key)}
        >{l.label}</button>
      ))}
    </div>
  );
}

export function ModelsSection({ s, busy, apply, onManageProviders }: SectionProps & { onManageProviders: () => void }) {
  const t = useT();
  const defaultRef = toRef(s.defaultModel, s);
  const [defaultProvider, defaultModel] = defaultRef.split("/");
  const [skillsOpen, setSkillsOpen] = useState(false);
  const subagentModels = s.subagentModels || {};

  const allRefs = s.providers.flatMap((p: any) => (p.models || []).map((m: string) => p.name + "/" + m));

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold px-1 pb-3">{t("settings.tab.models")}</div>

      <ModelCard icon={<Cpu size={18} />} title="默认执行模型 (Hephaestus)" desc="执行代码修改、运行命令等所有写操作">
        <ModelPicker
          s={s}
          refs={allRefs}
          value={defaultRef}
          disabled={busy}
          onPick={(ref: string) => void apply(() => app.SetDefaultModel(ref))}
        />
        <div className="mt-2">
          <EffortSelect
            value={s.agent.effort}
            busy={busy}
            onChange={(e: string) => void apply(() => app.SetEffort(e))}
          />
        </div>
      </ModelCard>

      <ModelCard icon={<Brain size={18} />} title="规划模型 (Hermes)" desc="只读研究代码、制定执行计划。留空则使用单模型模式">
        <ModelPicker
          s={s}
          refs={allRefs}
          value={s.plannerModel || ""}
          disabled={busy}
          emptyOptionLabel={t("settings.plannerNone")}
          onPick={(ref: string) => void apply(() => app.SetPlannerModel(ref))}
        />
        <div className="mt-2">
          <EffortSelect
            value={s.agent.plannerEffort ?? s.agent.effort}
            busy={busy}
            onChange={(e: string) => void apply(() => app.SetPlannerEffort(e))}
          />
        </div>
      </ModelCard>

      <ModelCard icon={<Bot size={18} />} title="子代理模型" desc="task / explore / review 等子任务使用的模型">
        <ModelPicker
          s={s}
          refs={allRefs}
          value={s.subagentModel || ""}
          disabled={busy}
          emptyOptionLabel={t("settings.subagentInherit")}
          onPick={(ref: string) => void apply(() => app.SetSubagentModel(ref))}
        />
        <div className="mt-2">
          <EffortSelect
            value={s.agent.subagentEffort ?? s.agent.effort}
            busy={busy}
            onChange={(e: string) => void apply(() => app.SetSubagentEffort(e))}
          />
        </div>
        {(s.subagentSkills || []).length > 0 && (
          <div className="mt-2">
            <button
              className="flex items-center gap-1 text-fg-dim text-[11px] font-medium hover:text-fg cursor-pointer bg-transparent border-0 p-0"
              onClick={() => setSkillsOpen((v) => !v)}
            >
              {skillsOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              {t("settings.subagentPerSkill") || "按技能单独配置"}
            </button>
            {skillsOpen && (
              <div className="mt-1.5 space-y-1.5 pl-4 border-l-2 border-border-soft">
                {s.subagentSkills.map((skill: string) => {
                  const skillRef = subagentModels[skill] || "";
                  const globalRef = s.subagentModel;
                  const inheritText = globalRef ? `继承全局: ${globalRef}` : t("settings.subagentInherit");
                  return (
                    <div key={skill} className="flex items-center gap-2">
                      <label className="text-fg-dim text-[11px] w-[90px] shrink-0 font-mono">{skill}</label>
                      <div className="flex-1">
                        <ModelPicker
                          s={s}
                          refs={allRefs}
                          value={skillRef}
                          disabled={busy}
                          emptyOptionLabel={inheritText}
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
      </ModelCard>

      {/* Step limits */}
      <div className="px-3 py-2 mb-3 border border-border-soft rounded-lg">
        <div className="text-fg-dim text-[11px] font-semibold uppercase tracking-wide mb-2">步数限制</div>
        <div className="flex items-center gap-4">
          <div className="flex-1">
            <div className="text-fg-faint text-[11px] mb-1">规划器步数</div>
            <StepLimitControl
              value={s.agent.plannerMaxSteps || 0}
              presets={[6, 12, 25, 0]}
              busy={busy}
              onChange={(n) => void apply(() => app.SetPlannerMaxSteps(n))}
            />
          </div>
          <div className="flex-1">
            <div className="text-fg-faint text-[11px] mb-1">执行器步数</div>
            <StepLimitControl
              value={s.agent.maxSteps || 0}
              presets={[10, 25, 50, 0]}
              busy={busy}
              onChange={(n) => void apply(() => app.SetAgentParams(s.agent.temperature, n, s.agent.systemPrompt))}
            />
          </div>
        </div>
      </div>

      <div className="flex items-center gap-2 px-3 py-2 border border-border-soft rounded-lg">
        <span className="text-fg-faint text-[11px] shrink-0">当前: {defaultProvider || t("common.none")} · {defaultModel || defaultRef || t("common.none")}</span>
        <span className="flex-1" />
        <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={onManageProviders}>
          {t("settings.manageProviders")}
        </button>
      </div>
    </section>
  );
}
