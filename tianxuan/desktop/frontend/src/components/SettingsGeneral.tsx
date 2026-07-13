import { useState } from "react";
import type { SectionProps } from "./SettingsShared";
import { SettingsPageShell, SettingsSection, SettingsField, SegmentedButton } from "./SettingsPageShell";
import { app } from "../lib/bridge";

// SettingsGeneral contains Agent runtime parameters: auto-plan, reasoning language,
// planner/executor max steps, sub-agent depth, cold resume prune, and output style.
export function SettingsGeneral({ s, busy: _busy, apply }: SectionProps) {
  const [depth, setDepth] = useState(s.agent.maxSubagentDepth);
  const [plannerSteps, setPlannerSteps] = useState(s.agent.plannerMaxSteps || 0);
  const [reasoningLang, setReasoningLang] = useState(s.agent.reasoningLanguage || "auto");
  const [autoPlan, setAutoPlan] = useState(s.agent.autoPlan || "off");
  const [outputStyle, setOutputStyle] = useState(s.agent.outputStyle || "");
  const [coldResume, setColdResume] = useState(s.agent.coldResumePrune);

  return (
    <SettingsPageShell title="通用" desc="智能体运行时行为与偏好设置。">
      <SettingsSection title="规划">
        <SettingsField label="自动规划" hint="多步任务自动启用规划模式。off=手动 / ask=询问 / on=自动。">
          <SegmentedButton
            options={[
              { value: "off", label: "关闭" },
              { value: "ask", label: "询问" },
              { value: "on", label: "开启" },
            ]}
            value={autoPlan}
            onChange={(v) => { setAutoPlan(v); void apply(() => app.SetAutoPlan(v)); }}
          />
        </SettingsField>

        <SettingsField label="规划器最大步数" hint="规划阶段工具调用轮数上限。0 = 不限。">
          <SegmentedButton
            options={[
              { value: "6", label: "6" },
              { value: "12", label: "12" },
              { value: "25", label: "25" },
              { value: "0", label: "∞" },
            ]}
            value={String(plannerSteps)}
            onChange={(v) => {
              const n = Number(v);
              setPlannerSteps(n);
              void apply(() => app.SetPlannerMaxSteps(n));
            }}
          />
        </SettingsField>
      </SettingsSection>

      <SettingsSection title="子代理">
        <SettingsField label="递归深度限制" hint="限制子代理嵌套层数。0 = 不限。">
          <SegmentedButton
            options={[
              { value: "0", label: "不限" },
              { value: "1", label: "1" },
              { value: "2", label: "2" },
              { value: "3", label: "3" },
            ]}
            value={String(depth)}
            onChange={(v) => {
              const n = Number(v);
              setDepth(n);
              void apply(() => app.SetMaxSubagentDepth(n));
            }}
          />
        </SettingsField>
      </SettingsSection>

      <SettingsSection title="推理">
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

      <SettingsSection title="上下文">
        <SettingsField label="冷恢复修剪" hint="冷启动恢复时自动移除过期工具结果以节省上下文。">
          <SegmentedButton
            options={[
              { value: "false", label: "关闭" },
              { value: "true", label: "开启" },
            ]}
            value={String(coldResume)}
            onChange={(v) => {
              const on = v === "true";
              setColdResume(on);
              void apply(() => app.SetColdResumePrune(on));
            }}
          />
        </SettingsField>
      </SettingsSection>
    </SettingsPageShell>
  );
}
