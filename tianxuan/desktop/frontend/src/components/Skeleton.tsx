import { useI18n } from "../lib/i18n";
import { Blocks, Cpu, Brain, Wrench } from "lucide-react";

export function Skeleton() {
  const { t } = useI18n();

  return (
    <div className="flex-1 px-8 py-10 flex flex-col items-center justify-center gap-8 overflow-hidden">
      {/* Animated pulse icon */}
      <div className="w-14 h-14 rounded-[14px] bg-accent/20 animate-pulse flex items-center justify-center">
        <Cpu size={28} className="text-accent/60" />
      </div>

      <div className="text-center max-w-md">
        <h2 className="text-[15px] font-semibold text-fg mb-2">{t("skeleton.title")}</h2>
        <p className="text-[13px] text-fg-dim leading-relaxed">{t("skeleton.desc")}</p>
      </div>

      {/* Capability cards */}
      <div className="grid grid-cols-2 gap-3 w-full max-w-sm">
        <div className="flex items-center gap-2.5 px-3 py-2.5 bg-bg-soft border border-border-soft rounded-lg">
          <Wrench size={16} className="text-fg-faint shrink-0" />
          <div className="flex flex-col">
            <span className="text-[13px] font-medium text-fg">{t("skeleton.tools")}</span>
            <span className="text-[11px] text-fg-faint">{t("skeleton.toolsDesc")}</span>
          </div>
        </div>
        <div className="flex items-center gap-2.5 px-3 py-2.5 bg-bg-soft border border-border-soft rounded-lg">
          <Brain size={16} className="text-fg-faint shrink-0" />
          <div className="flex flex-col">
            <span className="text-[13px] font-medium text-fg">{t("skeleton.skills")}</span>
            <span className="text-[11px] text-fg-faint">{t("skeleton.skillsDesc")}</span>
          </div>
        </div>
        <div className="flex items-center gap-2.5 px-3 py-2.5 bg-bg-soft border border-border-soft rounded-lg">
          <Blocks size={16} className="text-fg-faint shrink-0" />
          <div className="flex flex-col">
            <span className="text-[13px] font-medium text-fg">{t("skeleton.models")}</span>
            <span className="text-[11px] text-fg-faint">{t("skeleton.modelsDesc")}</span>
          </div>
        </div>
        <div className="flex items-center gap-2.5 px-3 py-2.5 bg-bg-soft border border-border-soft rounded-lg">
          <Cpu size={16} className="text-fg-faint shrink-0" />
          <div className="flex flex-col">
            <span className="text-[13px] font-medium text-fg">{t("skeleton.cache")}</span>
            <span className="text-[11px] text-fg-faint">{t("skeleton.cacheDesc")}</span>
          </div>
        </div>
      </div>

      {/* Animated dots */}
      <div className="flex gap-1.5">
        <span className="w-2 h-2 bg-accent/40 rounded-full animate-bounce" style={{ animationDelay: "0ms" }} />
        <span className="w-2 h-2 bg-accent/40 rounded-full animate-bounce" style={{ animationDelay: "150ms" }} />
        <span className="w-2 h-2 bg-accent/40 rounded-full animate-bounce" style={{ animationDelay: "300ms" }} />
      </div>
    </div>
  );
}
