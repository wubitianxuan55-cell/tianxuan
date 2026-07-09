import { useState } from "react";
import type { ColorScheme, ThemeMode } from "../lib/theme";

const SCHEME_META: Record<ColorScheme, { accent: string; label: string }> = {
  default: { accent: "#61AFEF", label: "默认" },
  warm:    { accent: "#D79921", label: "暖色" },
  ice:     { accent: "#88C0D0", label: "冰蓝" },
  forest:  { accent: "#83C092", label: "森林" },
  sunset:  { accent: "#FFA759", label: "日落" },
  ocean:   { accent: "#89B4FA", label: "海洋" },
  rose:    { accent: "#EB6F92", label: "玫瑰" },
  violet:  { accent: "#9D7CD8", label: "紫罗兰" },
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
          className="inline-block w-4 h-4 rounded border border-border-soft shrink-0"
          style={{ background: SCHEME_META[scheme].accent }}
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
            {SCHEMES.map((s) => (
              <button
                key={s}
                className={`w-full flex items-center gap-2 px-3 py-1.5 text-left bg-transparent border-0 text-fg-dim text-[12px] cursor-pointer transition-colors hover:bg-bg-soft hover:text-fg ${
                  scheme === s ? "text-accent bg-accent-soft" : ""
                }`}
                onClick={() => { onScheme(s); setOpen(false); }}
              >
                <span
                  className="inline-block w-3 h-3 rounded-full shrink-0 border border-border-soft"
                  style={{ background: SCHEME_META[s].accent }}
                />
                {SCHEME_META[s].label}
              </button>
            ))}

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
