import { ArrowUp } from "lucide-react";
import { FolderOpen, Bug, Lightbulb } from "lucide-react";
import { useCallback, useRef, useState } from "react";
import logo from "../assets/logo.png";
import { useT } from "../lib/i18n";

interface StarterCard {
  icon: React.ReactNode;
  title: string;
  desc: string;
  prompt: string;
}

export function Welcome({ onPrompt }: { onPrompt: (text: string) => void }) {
  const t = useT();
  const [text, setText] = useState("");
  const taRef = useRef<HTMLTextAreaElement>(null);

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

  const handleSubmit = useCallback(() => {
    const trimmed = text.trim();
    if (!trimmed) return;
    onPrompt(trimmed);
    setText("");
  }, [text, onPrompt]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  return (
    <div className="h-full flex flex-col items-center justify-center max-w-2xl mx-auto px-6 overflow-y-auto">
      {/* Logo */}
      <img src={logo} className="w-10 h-10 rounded-[10px] mb-3" alt="tianxuan" />
      <div className="text-[13.5px] text-fg-dim mb-7">{t("welcome.tagline")}</div>

      {/* Central input box */}
      <div className="w-full border border-border rounded-xl bg-bg-soft shadow-sm hover:border-fg-faint focus-within:border-accent transition-colors duration-[0.15s]">
        <textarea
          ref={taRef}
          className="w-full resize-none border-0 bg-transparent text-fg text-[14px] leading-relaxed outline-none placeholder:text-fg-faint px-4 pt-4 pb-2"
          style={{ minHeight: "64px", maxHeight: "160px" }}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={t("composer.placeholder")}
          rows={2}
        />
        <div className="flex items-center justify-between px-3 pb-3">
          <span className="text-[11px] text-fg-faint">
            <kbd className="font-mono text-fg-dim bg-bg-elev-2 border border-border rounded px-1 py-px text-[10px]">/</kbd> 命令
            <span className="mx-1.5">·</span>
            <kbd className="font-mono text-fg-dim bg-bg-elev-2 border border-border rounded px-1 py-px text-[10px]">@</kbd> 文件
            <span className="mx-1.5">·</span>
            <kbd className="font-mono text-fg-dim bg-bg-elev-2 border border-border rounded px-1 py-px text-[10px]">↵</kbd> 发送
          </span>
          <button
            className={`inline-flex items-center justify-center w-7 h-7 border-0 rounded-md cursor-pointer shrink-0 transition-all duration-[0.12s] active:scale-95 ${
              text.trim()
                ? "bg-accent text-accent-fg hover:brightness-110"
                : "bg-bg-elev-2 text-fg-faint"
            }`}
            onClick={handleSubmit}
            disabled={!text.trim()}
          >
            <ArrowUp size={15} />
          </button>
        </div>
      </div>

      {/* Starter cards grid */}
      <div className="grid grid-cols-3 gap-2.5 mt-6 w-full">
        {cards.map((card) => (
          <button
            key={card.title}
            className="flex flex-col items-start gap-2 text-left font-[inherit] text-[13px] bg-bg-soft border border-border-soft text-fg-dim rounded-lg p-3 hover:text-fg hover:border-accent-soft hover:bg-bg-elev transition-colors"
            onClick={() => onPrompt(card.prompt)}
          >
            <span className="text-fg-dim">{card.icon}</span>
            <span className="flex flex-col gap-0.5">
              <span className="text-[13px] font-medium text-fg">{card.title}</span>
              <span className="text-[12px] text-fg-faint leading-snug">{card.desc}</span>
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
