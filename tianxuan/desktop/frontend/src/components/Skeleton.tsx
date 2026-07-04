import { memo } from "react";
import { useI18n } from "../lib/i18n";
import { Blocks, Cpu, Brain, Wrench, Zap } from "lucide-react";

export const Skeleton = memo(function Skeleton() {
  const { t } = useI18n();

  return (
    <div className="flex-1 px-8 py-10 flex flex-col items-center justify-center gap-8 overflow-hidden relative">
      {/* subtle radial glow behind the icon */}
      <div className="absolute top-[10%] left-1/2 -translate-x-1/2 w-80 h-80 rounded-full bg-accent/[0.04] blur-3xl pointer-events-none" />

      {/* Animated layered rings + glowing icon */}
      <div className="relative w-20 h-20 flex items-center justify-center">
        {/* outer glow ring */}
        <span className="absolute inset-[-6px] rounded-2xl bg-accent/15 animate-pulse" />
        {/* spinning ring 1 (slow, large) */}
        <span className="absolute inset-[-10px] rounded-2xl border-2 border-accent/25 animate-[spin_12s_linear_infinite]" />
        {/* spinning ring 2 (faster, smaller) */}
        <span className="absolute inset-[-4px] rounded-xl border border-accent/35 animate-[spin_6s_linear_infinite_reverse]" />
        {/* icon with glow */}
        <Zap size={36} className="text-accent relative z-10" style={{ filter: "drop-shadow(0 0 12px var(--accent))" }} />
      </div>

      <div className="text-center max-w-md">
        <h2 className="text-[15px] font-semibold text-fg mb-2">{t("skeleton.title")}</h2>
        <p className="text-[13px] text-fg-dim leading-relaxed">{t("skeleton.desc")}</p>
      </div>

      {/* Capability cards */}
      <div className="grid grid-cols-2 gap-3 w-full max-w-sm">
        <Card icon={<Wrench size={16} />} color="var(--accent)"      title={t("skeleton.tools")}  desc={t("skeleton.toolsDesc")} />
        <Card icon={<Brain size={16} />}   color="#a78bfa"            title={t("skeleton.skills")} desc={t("skeleton.skillsDesc")} />
        <Card icon={<Blocks size={16} />}  color="#38bdf8"            title={t("skeleton.models")} desc={t("skeleton.modelsDesc")} />
        <Card icon={<Cpu size={16} />}     color="#34d399"            title={t("skeleton.cache")}  desc={t("skeleton.cacheDesc")} />
      </div>

      {/* Animated dots */}
      <div className="flex gap-1.5">
        <span className="w-2 h-2 bg-accent/50 rounded-full animate-bounce [animation-delay:0ms]" />
        <span className="w-2 h-2 bg-accent/50 rounded-full animate-bounce [animation-delay:150ms]" />
        <span className="w-2 h-2 bg-accent/50 rounded-full animate-bounce [animation-delay:300ms]" />
      </div>
    </div>
  );
});

function Card({ icon, title, desc, color }: { icon: React.ReactNode; title: string; desc: string; color: string }) {
  return (
    <div className="flex items-center gap-2.5 px-3 py-2.5 bg-bg-elev border border-border-soft rounded-lg transition-[border-color,background] duration-[var(--dur-fast)] hover:border-fg-faint/40">
      <span className="shrink-0" style={{ color }}>{icon}</span>
      <div className="flex flex-col min-w-0">
        <span className="text-[13px] font-medium text-fg truncate">{title}</span>
        <span className="text-[11px] text-fg-faint truncate">{desc}</span>
      </div>
    </div>
  );
}
