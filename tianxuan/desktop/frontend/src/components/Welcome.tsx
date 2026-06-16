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
    <div className="welcome">
      <img src={logo} className="welcome__logo" alt="tianxuan" />
      <div className="welcome__title">tianxuan</div>
      <div className="welcome__tag">{t("welcome.tagline")}</div>

      <div className="welcome__hints">
        <span>
          <kbd>/</kbd> {t("welcome.hintCommands")}
        </span>
        <span>
          <kbd>@</kbd> {t("welcome.hintFiles")}
        </span>
        <span>
          <kbd>⏎</kbd> {t("welcome.hintSend")}
        </span>
      </div>

      {/* V5.16: 快捷任务卡片网格 */}
      <div className="welcome__cards">
        {cards.map((card) => (
          <button
            key={card.title}
            className="welcome__card"
            onClick={() => onPrompt(card.prompt)}
          >
            <span className="welcome__card-icon">{card.icon}</span>
            <span className="welcome__card-body">
              <span className="welcome__card-title">{card.title}</span>
              <span className="welcome__card-desc">{card.desc}</span>
            </span>
          </button>
        ))}
      </div>

      <div className="welcome__examples">
        <button className="welcome__ex" onClick={() => onPrompt(t("welcome.ex1"))}>
          {t("welcome.ex1")}
        </button>
      </div>
    </div>
  );
}
