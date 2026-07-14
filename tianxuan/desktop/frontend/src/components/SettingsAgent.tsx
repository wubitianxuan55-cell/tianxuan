import { useState } from "react";
import { Cpu, Brain, Bot, ChevronDown, ChevronRight, Settings as SettingsIcon } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { ModelPicker } from "./ModelPicker";
import { StepLimitControl } from "./StepLimitControl";
import { SettingsPageShell, SettingsSection, SettingsField, SegmentedButton } from "./SettingsPageShell";
import { toRef, type SectionProps } from "./SettingsShared";

// ── helpers ──

const EFFORT_LEVELS = [
  { key: "", label: "关闭", hint: "直接输出，不显示思考过程" },
  { key: "high", label: "标准", hint: "适度推理，日常任务推荐" },
  { key: "max", label: "深度", hint: "扩展推理链，复杂逻辑任务" },
] as const;

function EffortSelect({ value, onChange, busy }: { value: string; onChange: (e: string) => void; busy: boolean }) {
  const v = value ?? "";
  return (
    <div className="flex items-center gap-1">
      <span className="text-fg-faint text-[11px] shrink-0 mr-0.5">思考深度</span>
      {EFFORT_LEVELS.map((l) => (
        <button
          key={l.key}
          className={`px-2 py-0.5 text-[11px] border rounded transition-colors ${
            v === l.key
              ? "text-accent border-accent bg-accent/15 font-semibold ring-2 ring-accent/50 shadow-sm"
              : "text-fg-dim border-border-soft bg-bg-soft/60 hover:bg-bg-soft hover:text-fg hover:border-border"
          }`}
          disabled={busy}
          title={l.hint}
          onClick={() => onChange(l.key)}
        >{l.label}</button>
      ))}
    </div>
  );
}

function ModelCard({ icon, title, desc, children }: { icon: React.ReactNode; title: string; desc: string; children: React.ReactNode }) {
  return (
    <div className="bg-bg border border-border rounded-lg p-3.5">
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

// ── main ──

export function AgentSection({ s, busy, apply }: SectionProps) {
  const t = useT();
  const [skillsOpen, setSkillsOpen] = useState(false);
  const subagentModels = s.subagentModels || {};

  // state for system prompt + temperature
  const [temp, setTemp] = useState(String(s.agent.temperature));
  const [prompt, setPrompt] = useState(s.agent.systemPrompt);
  const dirty = temp !== String(s.agent.temperature) || prompt !== s.agent.systemPrompt;

  // local state for toggle fields
  const [reasoningLang, setReasoningLang] = useState(s.agent.reasoningLanguage || "auto");
  const [outputStyle, setOutputStyle] = useState(s.agent.outputStyle || "");

  const allRefs = s.providers.flatMap((p: any) => (p.models || []).map((m: string) => p.name + "/" + m));
  const defaultRef = toRef(s.defaultModel, s);

  return (
    <SettingsPageShell title={t("settings.tab.agent")} desc="配置模型、子代理、推理参数与系统提示词。">
      <div className="space-y-5">
        {/* ── 模型配置 ── */}
        <SettingsSection title={<span className="flex items-center gap-1.5"><Cpu size={14} className="text-accent" />模型配置</span>}>
          <div className="grid grid-cols-2 gap-3">
            <ModelCard icon={<Cpu size={18} />} title="默认执行模型 (Hephaestus)" desc="执行代码修改、运行命令等所有写操作。">
              <ModelPicker
                s={s} refs={allRefs} value={defaultRef} disabled={busy}
                onPick={(ref: string) => void apply(() => app.SetDefaultModel(ref))}
              />
              <div className="mt-2">
                <EffortSelect
                  value={s.agent.effort} busy={busy}
                  onChange={(e: string) => void apply(() => app.SetEffort(e))}
                />
              </div>
            </ModelCard>

            <ModelCard icon={<Brain size={18} />} title="规划模型 (Hermes)" desc="只读研究代码、制定执行计划。留空则使用单模型模式。">
              <ModelPicker
                s={s} refs={allRefs} value={s.plannerModel || ""} disabled={busy}
                emptyOptionLabel={t("settings.plannerNone")}
                onPick={(ref: string) => void apply(() => app.SetPlannerModel(ref))}
              />
              <div className="mt-2">
                <EffortSelect
                  value={s.agent.plannerEffort ?? s.agent.effort} busy={busy}
                  onChange={(e: string) => void apply(() => app.SetPlannerEffort(e))}
                />
              </div>
            </ModelCard>

            <div className="col-span-2">
            <ModelCard icon={<Bot size={18} />} title="子代理模型" desc="task / explore / review 等子任务使用的模型。">
              <ModelPicker
                s={s} refs={allRefs} value={s.subagentModel || ""} disabled={busy}
                emptyOptionLabel={t("settings.subagentInherit")}
                onPick={(ref: string) => void apply(() => app.SetSubagentModel(ref))}
              />
              <div className="mt-2">
                <EffortSelect
                  value={s.agent.subagentEffort ?? s.agent.effort} busy={busy}
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
                        const inheritText = s.subagentModel
                          ? `继承全局: ${s.subagentModel}`
                          : t("settings.subagentInherit");
                        return (
                          <div key={skill} className="flex items-center gap-2">
                            <label className="text-fg-dim text-[11px] w-[90px] shrink-0 font-mono">{skill}</label>
                            <div className="flex-1">
                              <ModelPicker
                                s={s} refs={allRefs} value={skillRef} disabled={busy}
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
            </div>
          </div>
        </SettingsSection>

        {/* ── 步数与推理 ── */}
        <SettingsSection title={<span className="flex items-center gap-1.5"><SettingsIcon size={14} className="text-accent" />步数与推理</span>}>
          <SettingsField label="规划器步数" hint="规划阶段工具调用轮数上限。0 = 不限。">
            <StepLimitControl
              value={s.agent.plannerMaxSteps || 0}
              presets={[6, 12, 25, 0]}
              busy={busy}
              onChange={(n) => void apply(() => app.SetPlannerMaxSteps(n))}
            />
          </SettingsField>
          <SettingsField label="执行器步数" hint="执行阶段工具调用轮数上限。0 = 不限。">
            <StepLimitControl
              value={s.agent.maxSteps || 0}
              presets={[10, 25, 50, 0]}
              busy={busy}
              onChange={(n) => void apply(() => app.SetAgentParams(s.agent.temperature, n, s.agent.systemPrompt))}
            />
          </SettingsField>
          <SettingsField label="温度" hint="控制输出随机性。越高越有创意，越低越稳定。">
            <div className="flex items-center gap-2">
              <SegmentedButton
                options={[
                  { value: "0", label: "0" },
                  { value: "0.3", label: "0.3" },
                  { value: "0.7", label: "0.7" },
                  { value: "1", label: "1.0" },
                ]}
                value={temp}
                onChange={(v) => setTemp(v)}
              />
            </div>
          </SettingsField>
          <SettingsField label="推理语言" hint="控制模型思考文本的语言偏好。">
            <SegmentedButton
              options={[
                { value: "auto", label: "自动" },
                { value: "zh", label: "中文" },
                { value: "en", label: "English" },
              ]}
              value={reasoningLang}
              onChange={(v) => { setReasoningLang(v); void apply(() => app.SetReasoningLanguage(v)); }}
            />
          </SettingsField>
          <SettingsField label="输出风格" hint="影响智能体回复的语气和详细程度。">
            <SegmentedButton
              options={[
                { value: "", label: "默认" },
                { value: "concise", label: "简洁" },
                { value: "explanatory", label: "详细" },
              ]}
              value={outputStyle}
              onChange={(v) => { setOutputStyle(v); void apply(() => app.SetOutputStyle(v)); }}
            />
          </SettingsField>
        </SettingsSection>

        {/* ── 系统提示词 ── */}
        <SettingsSection title={<span className="flex items-center gap-1.5"><SettingsIcon size={14} className="text-accent" />系统提示词</span>}>
          <div className="text-fg-faint text-[11px] mb-1.5">自定义系统提示词，覆盖默认的智能体行为指令。留空则使用内置模板。</div>
          <textarea
            className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] p-2.5 outline-none resize-y min-h-[140px] focus:border-accent"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            disabled={busy}
            spellCheck={false}
          />
          <div className="flex items-center justify-between mt-1.5">
            <span className="text-fg-faint text-[10px]">{prompt.length} 字符</span>
            <button
              className="btn btn--primary btn--small"
              disabled={busy || !dirty}
              onClick={() => void apply(async () => {
                // apply all local edits before saving params
                if (temp !== String(s.agent.temperature) || prompt !== s.agent.systemPrompt) {
                  await app.SetAgentParams(Number(temp) || 0, s.agent.maxSteps, prompt);
                }
              })}
            >
              {t("settings.saveAgent")}
            </button>
          </div>
        </SettingsSection>
      </div>
    </SettingsPageShell>
  );
}
