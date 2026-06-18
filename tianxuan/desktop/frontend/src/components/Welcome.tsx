import { FolderOpen, Bug, Lightbulb } from "lucide-react";
import logo from "../assets/logo.png";
import { useT } from "../lib/i18n";

// V5.16: DeepSeek-GUI 风格快捷任务卡片 (Kun ChatStarterGrid 移植)

interface StarterCard {
  icon: React.ReactNode;
  title: string;
  desc: string;
  prompt: string;
}

export function Welcome({ onPrompt }: { onPrompt: (text: string) => void }) {
  const t = useT();

  const cards: StarterCard[] = [
    {
      icon: <FolderOpen size={18} />,
      title: t("welcome.card1.title"),
      desc: t("welcome.card1.desc"),
      prompt: t("welcome.card1.prompt"),
    },
    {
      icon: <Bug size={18} />,
      title: t("welcome.card2.title"),
      desc: t("welcome.card2.desc"),
      prompt: t("welcome.card2.prompt"),
    },
    {
      icon: <Lightbulb size={18} />,
      title: t("welcome.card3.title"),
      desc: t("welcome.card3.desc"),
      prompt: t("welcome.card3.prompt"),
    },
  ];

  return (
    <div className="h-full flex flex-col items-center justify-center max-w-lg mx-auto px-4 overflow-y-auto">
      <img src={logo} className="w-[54px] h-[54px] mb-4 rounded-[13px]" alt="tianxuan" />
      <div className="text-[22px] font-semibold tracking-[0.3px] text-(--color-fg)">tianxuan</div>
      <div className="mt-1.5 text-[13.5px] text-(--color-fg-dim)">{t("welcome.tagline")}</div>

      {/* Hints */}
      <div className="flex gap-4 mt-[18px] text-[12.5px]">
        <span className="inline-flex items-center gap-1.5">
          <kbd className="font-mono text-[11px] text-(--color-fg-dim) bg-(--color-bg-elev-2) border border-(--color-border) rounded px-1.5 py-px">/</kbd>
          {t("welcome.hintCommands")}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <kbd className="font-mono text-[11px] text-(--color-fg-dim) bg-(--color-bg-elev-2) border border-(--color-border) rounded px-1.5 py-px">@</kbd>
          {t("welcome.hintFiles")}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <kbd className="font-mono text-[11px] text-(--color-fg-dim) bg-(--color-bg-elev-2) border border-(--color-border) rounded px-1.5 py-px">⏎</kbd>
          {t("welcome.hintSend")}
        </span>
      </div>

      {/* Starter cards grid */}
      <div className="grid grid-cols-3 gap-2.5 mt-[22px] w-full">
        {cards.map((card) => (
          <button
            key={card.title}
            className="flex flex-col items-start gap-2 text-left font-[inherit] text-[13px] bg-(--color-bg-soft) border border-(--color-border-soft) text-(--color-fg-dim) rounded-lg p-3 hover:text-(--color-fg) hover:border-(--color-accent-soft) hover:bg-(--color-bg-elev) transition-colors"
            onClick={() => onPrompt(card.prompt)}
          >
            <span className="text-(--color-fg-dim)">{card.icon}</span>
            <span className="flex flex-col gap-0.5">
              <span className="text-[13px] font-medium text-(--color-fg)">{card.title}</span>
              <span className="text-[12px] text-(--color-fg-faint) leading-snug">{card.desc}</span>
            </span>
          </button>
        ))}
      </div>

      {/* Examples */}
      <div className="flex flex-col gap-2 mt-[26px] w-full">
        <button
          className="text-left bg-(--color-bg-soft) border border-(--color-border-soft) text-(--color-fg-dim) font-[inherit] text-[13px] rounded-lg px-3 py-2 hover:text-(--color-fg) hover:border-(--color-accent-soft) hover:bg-(--color-bg-elev) transition-colors"
          onClick={() => onPrompt(t("welcome.ex1"))}
        >
          {t("welcome.ex1")}
        </button>
      </div>
    </div>
  );
}
