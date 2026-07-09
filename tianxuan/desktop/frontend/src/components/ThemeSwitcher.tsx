import { useState } from "react";
import type { ColorScheme, ThemeMode } from "../lib/theme";

const SCHEME_META: Record<ColorScheme, { dot: string; label: string }> = {
  default: { dot: "#22C55E", label: "默认" },
  warm:    { dot: "#F59E0B", label: "暖色" },
  ice:     { dot: "#38BDF8", label: "冰蓝" },
  forest:  { dot: "#4ADE80", label: "森林" },
  sunset:  { dot: "#F97316", label: "日落" },
  ocean:   { dot: "#14B8A6", label: "海洋" },
  rose:    { dot: "#EC4899", label: "玫瑰" },
  violet:  { dot: "#A855F7", label: "紫罗兰" },
};

const SCHEMES: ColorScheme[] = ["default", "warm", "ice", "forest", "sunset", "ocean", "rose", "violet"];

export function ThemeSwitcher({
  scheme,
  mode,
  onScheme,
  onMode,
}: {
  scheme: ColorScheme;
  mode: ThemeMode;
  onScheme: (s: ColorScheme) => void;
  onMode: (m: ThemeMode) => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div className="relative inline-flex no-drag">
      <button
        className="toolbar-btn no-drag"
        onClick={() => setOpen((v) => !v)}
        title="切换主题"
      >
        <span
          className="inline-block w-3 h-3 rounded-full border border-border-soft shrink-0"
          style={{ background: SCHEME_META[scheme].dot }}
        />
        <span>{SCHEME_META[scheme].label}</span>
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div
            className="absolute top-full right-0 mt-1 z-50 min-w-[200px] py-2 bg-bg-elev-2 border border-border rounded-lg"
            style={{ boxShadow: "var(--ds-shadow-dropdown)" }}
          >
            {/* ── 配色选择 ── */}
            <div className="px-2 mb-1 text-[10px] font-semibold text-fg-faint uppercase tracking-wider">配色</div>
            <div className="grid grid-cols-4 gap-1.5 px-2 mb-2">
              {SCHEMES.map((s) => (
                <button
                  key={s}
                  className={`flex flex-col items-center gap-0.5 p-1.5 rounded-md border-0 bg-transparent cursor-pointer transition-colors hover:bg-bg-soft ${
                    scheme === s ? "ring-1 ring-accent" : ""
                  }`}
                  onClick={() => onScheme(s)}
                  title={SCHEME_META[s].label}
                >
                  <span
                    className="inline-block w-4 h-4 rounded-full"
                    style={{ background: SCHEME_META[s].dot }}
                  />
                </button>
              ))}
            </div>

            {/* ── 模式选择 ── */}
            <div className="border-t border-border-soft pt-2 px-2">
              <div className="inline-flex w-full border border-border-soft rounded-md overflow-hidden">
                {([
                  { value: "light" as ThemeMode, icon: "☀️", label: "浅色" },
                  { value: "dark" as ThemeMode,  icon: "🌙", label: "深色" },
                  { value: "auto" as ThemeMode,  icon: "💻", label: "自动" },
                ]).map(({ value, icon, label }) => (
                  <button
                    key={value}
                    className={`flex-1 flex items-center justify-center gap-1 py-1.5 bg-transparent border-0 text-fg-dim text-[11px] cursor-pointer transition-colors hover:text-fg hover:bg-bg-soft ${
                      mode === value ? "bg-accent-soft text-accent" : ""
                    }`}
                    onClick={() => onMode(value)}
                  >
                    <span className="text-xs">{icon}</span>
                    <span>{label}</span>
                  </button>
                ))}
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
