import { useI18n } from "../lib/i18n";
import { Blocks, Cpu, Brain, Wrench } from "lucide-react";

export function Skeleton() {
  const { t } = useI18n();

  return (
    <div className="flex-1 px-8 py-10 flex flex-col items-center justify-center gap-8 overflow-hidden">
      {/* Animated pulse icon — rotating outer ring */}
      <div className="relative w-16 h-16 flex items-center justify-center">
        <span className="absolute inset-0 rounded-2xl bg-accent/10 animate-pulse" />
        <span className="absolute inset-[-2px] rounded-2xl border border-accent/20 animate-[spin_8s_linear_infinite]" />
        <Cpu size={30} className="text-accent/50 relative z-10" />
      </div>

      <div className="text-center max-w-md">
        <h2 className="text-[15px] font-semibold text-fg mb-2">{t("skeleton.title")}</h2>
        <p className="text-[13px] text-fg-dim leading-relaxed">{t("skeleton.desc")}</p>
      </div>

      {/* Capability cards */}
      <div className="grid grid-cols-2 gap-3 w-full max-w-sm">
        <Card icon={<Wrench size={15} />} title={t("skeleton.tools")} desc={t("skeleton.toolsDesc")} />
        <Card icon={<Brain size={15} />} title={t("skeleton.skills")} desc={t("skeleton.skillsDesc")} />
        <Card icon={<Blocks size={15} />} title={t("skeleton.models")} desc={t("skeleton.modelsDesc")} />
        <Card icon={<Cpu size={15} />} title={t("skeleton.cache")} desc={t("skeleton.cacheDesc")} />
      </div>

      {/* Animated dots */}
      <div className="flex gap-1.5">
        <span className="w-2 h-2 bg-accent/40 rounded-full animate-bounce [animation-delay:0ms]" />
        <span className="w-2 h-2 bg-accent/40 rounded-full animate-bounce [animation-delay:150ms]" />
        <span className="w-2 h-2 bg-accent/40 rounded-full animate-bounce [animation-delay:300ms]" />
      </div>
    </div>
  );
}

function Card({ icon, title, desc }: { icon: React.ReactNode; title: string; desc: string }) {
  return (
    <div className="flex items-center gap-2.5 px-3 py-2.5 bg-bg-soft border border-border-soft rounded-lg transition-[border-color,background] duration-[var(--dur-fast)] hover:border-fg-faint hover:bg-bg-elev">
      <span className="text-fg-faint shrink-0">{icon}</span>
      <div className="flex flex-col min-w-0">
        <span className="text-[13px] font-medium text-fg truncate">{title}</span>
        <span className="text-[11px] text-fg-faint truncate">{desc}</span>
      </div>
    </div>
  );
}
