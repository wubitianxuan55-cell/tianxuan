import { useState } from "react";
import type { Theme } from "../lib/theme";

const THEME_DOTS: Record<string, string> = {
  dark: "#0b0f15",
  light: "#f8f6f2",
  warm: "#1e1814",
  ice: "#0a111a",
  forest: "#0d1510",
};

const THEME_NAMES: Record<string, string> = {
  dark: "深色",
  light: "浅色",
  warm: "暖色",
  ice: "冰蓝",
  forest: "森林",
  auto: "自动",
};

export function ThemeSwitcher({
  theme,
  onSet,
  onStore,
}: {
  theme: string;
  onSet: (theme: Theme) => void;
  onStore: (theme: Theme) => void;
}) {
  const [open, setOpen] = useState(false);
  const themes: Theme[] = ["dark", "light", "warm", "ice", "forest", "auto"];
  const current = theme === "auto" ? "auto" : theme;

  const handlePick = (th: Theme) => {
    onStore(th);
    onSet(th);
    setOpen(false);
  };

  return (
    <div className="relative inline-flex no-drag">
      <button
        className="toolbar-btn no-drag"
        onClick={() => setOpen((v) => !v)}
        title="切换主题"
      >
        <span
          className="inline-block w-3 h-3 rounded-full border border-border-soft shrink-0"
          style={{ background: THEME_DOTS[current] ?? THEME_DOTS.dark }}
        />
        <span>{THEME_NAMES[current] ?? current}</span>
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute top-full right-0 mt-1 z-50 min-w-[120px] py-1 max-h-[320px] overflow-y-auto bg-bg-elev-2 border border-border rounded-lg" style={{boxShadow: "var(--ds-shadow-dropdown)"}}>
            {themes.map((th) => (
              <button
                key={th}
                className={`w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg flex items-center gap-2 ${
                  current === th ? "text-accent" : ""
                }`}
                onClick={() => handlePick(th)}
              >
                <span
                  className={`inline-block w-3 h-3 rounded-full shrink-0 ${
                    current === th ? "ring-2 ring-accent ring-offset-1 ring-offset-bg-elev-2" : ""
                  }`}
                  style={{ background: THEME_DOTS[th] ?? "#555" }}
                />
                <span>{THEME_NAMES[th] ?? th}</span>
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
