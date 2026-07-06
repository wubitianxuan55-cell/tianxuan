import { useState } from "react";
import { useI18n } from "../lib/i18n";
import type { Theme } from "../lib/theme";

function darkenHex(hex: string, amount: number): string {
  const n = parseInt(hex.slice(1), 16);
  const r = Math.max(0, Math.min(255, ((n >> 16) & 0xff) + amount));
  const g = Math.max(0, Math.min(255, ((n >> 8) & 0xff) + amount));
  const b = Math.max(0, Math.min(255, (n & 0xff) + amount));
  return ((r << 16) | (g << 8) | b).toString(16).padStart(6, "0");
}

function mixHex(a: string, b: string, t: number): string {
  const na = parseInt(a.slice(1), 16), nb = parseInt(b.slice(1), 16);
  const r = Math.round(((na >> 16) & 0xff) * (1 - t) + ((nb >> 16) & 0xff) * t);
  const g = Math.round(((na >> 8) & 0xff) * (1 - t) + ((nb >> 8) & 0xff) * t);
  const bl = Math.round((na & 0xff) * (1 - t) + (nb & 0xff) * t);
  return ((r << 16) | (g << 8) | bl).toString(16).padStart(6, "0");
}

export function AppearanceSection({ theme, onTheme }: { theme: Theme; onTheme: (t: Theme) => void }) {
  const { t, pref, setPref } = useI18n();
  const themeOptions: Theme[] = ["dark", "light", "warm", "ice", "forest"];

  // 字体偏好（localStorage + DOM attribute）
  const [uiFont, setUiFont] = useState(() => {
    try { return localStorage.getItem("tianxuan.uiFont") || ""; } catch { return ""; }
  });
  const [monoFont, setMonoFont] = useState(() => {
    try { return localStorage.getItem("tianxuan.monoFont") || ""; } catch { return ""; }
  });
  const applyFont = (kind: "ui" | "mono", value: string) => {
    const attr = kind === "ui" ? "data-font-family" : "data-mono-font-family";
    if (value) {
      document.documentElement.setAttribute(attr, value);
      localStorage.setItem(`tianxuan.${kind === "ui" ? "uiFont" : "monoFont"}`, value);
    } else {
      document.documentElement.removeAttribute(attr);
      localStorage.removeItem(`tianxuan.${kind === "ui" ? "uiFont" : "monoFont"}`);
    }
    if (kind === "ui") setUiFont(value); else setMonoFont(value);
  };

  const themeColors: Record<Theme, { bg: string; accent: string; fg: string; label: string }> = {
    auto:   { bg: "#0b0f15", accent: "#6ee7ff", fg: "#e6ebf2", label: t("settings.themeAuto") },
    dark:   { bg: "#0b0f15", accent: "#6ee7ff", fg: "#e6ebf2", label: t("settings.themeDark") },
    light:  { bg: "#f7f4ef", accent: "#3b82f6", fg: "#1f1d1a", label: t("settings.themeLight") },
    warm:   { bg: "#fdf6e3", accent: "#a855f7", fg: "#3d2b1f", label: "暖色" },
    ice:    { bg: "#0d1b2a", accent: "#6ee7ff", fg: "#e0e8f2", label: "冰蓝" },
    forest: { bg: "#0f1a0f", accent: "#4ade80", fg: "#e0ecd8", label: "森林" },
  };

  const tc = themeColors[theme] ?? themeColors.dark;

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold mb-3">{t("settings.appearance")}</div>

      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">{t("settings.theme")}</label>
        <div className="flex items-center gap-2 mb-3">
          <button
            className={`flex items-center gap-2 px-3 py-1.5 bg-transparent border border-border-soft rounded-md text-fg-dim text-xs cursor-pointer transition-all hover:text-fg hover:bg-bg-soft ${theme === "auto" ? "border-accent bg-accent-soft text-accent" : ""}`}
            onClick={() => onTheme("auto")}
          >
            <span className="w-3 h-3 rounded-full border border-fg-faint/30" style={{ background: "conic-gradient(#f4f5f7 0deg 180deg, #090a0c 180deg 360deg)" }} />
            {t("settings.themeAuto")}
          </button>
        </div>

        <div className="grid grid-cols-2 gap-2">
          {themeOptions.map((opt) => {
            const c = themeColors[opt];
            const isActive = theme === opt;
            return (
              <button
                key={opt}
                onClick={() => onTheme(opt)}
                className={`text-left bg-bg-soft border rounded-lg p-2.5 cursor-pointer transition-all hover:-translate-y-px hover:shadow-lg ${
                  isActive ? "border-accent ring-1 ring-accent/50" : "border-border-soft hover:border-fg-faint/30"
                }`}
              >
                <div className="flex gap-1 mb-2 rounded-md overflow-hidden h-6" style={{ background: c.bg }}>
                  <div className="flex-1" style={{ background: c.bg }} />
                  <div className="w-3" style={{ background: c.accent }} />
                  <div className="w-6 flex items-center justify-center text-[7px] font-mono" style={{ background: c.fg, color: c.bg }}>Aa</div>
                </div>
                <span className={`text-[11px] font-medium ${isActive ? "text-accent" : "text-fg-dim"}`}>{c.label}</span>
                {isActive && <span className="ml-1.5 text-[10px] text-accent">✓</span>}
              </button>
            );
          })}
        </div>
      </div>

      <div className="mb-4 p-3 bg-bg-soft border border-border-soft rounded-lg">
        <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mb-2">色板预览</div>
        <div className="flex flex-wrap gap-1.5">
          {[
            { label: "bg", color: tc.bg },
            { label: "bg-soft", color: "#" + darkenHex(tc.bg, -8) },
            { label: "bg-elev", color: "#" + darkenHex(tc.bg, -16) },
            { label: "accent", color: tc.accent },
            { label: "border", color: "#" + mixHex(tc.bg, tc.fg, 0.3) },
            { label: "fg", color: tc.fg },
            { label: "fg-dim", color: "#" + mixHex(tc.bg, tc.fg, 0.7) },
            { label: "ok", color: "#74b87a" },
            { label: "warn", color: "#d9a441" },
            { label: "err", color: "#e0696a" },
          ].map(({ label, color }) => (
            <div key={label} className="flex flex-col items-center gap-0.5">
              <div className="w-7 h-7 rounded-md border border-border-soft" style={{ background: color }} />
              <span className="text-[9px] text-fg-faint font-mono">{label}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">界面字体</label>
        <select className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={uiFont} onChange={e => applyFont("ui", e.target.value)}>
          <option value="">系统默认</option>
          <option value="pingfang">苹方 (PingFang SC)</option>
          <option value="yahei">微软雅黑</option>
          <option value="noto">Noto Sans SC</option>
        </select>
      </div>

      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">等宽字体</label>
        <select className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={monoFont} onChange={e => applyFont("mono", e.target.value)}>
          <option value="">系统默认</option>
          <option value="cascadia">Cascadia Code</option>
          <option value="jetbrains">JetBrains Mono</option>
          <option value="sfmono">SF Mono</option>
        </select>
      </div>

      <div>
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">{t("settings.language")}</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {[
            { value: "", label: t("settings.langAuto") },
            { value: "zh", label: "简体中文" },
            { value: "zh-TW", label: "繁體中文" },
            { value: "en", label: "English" },
          ].map(({ value, label }) => (
            <button
              key={value}
              className={`px-3 py-1.5 bg-transparent border-0 border-r border-border-soft text-fg-dim text-xs cursor-pointer transition-[color,background] hover:text-fg hover:bg-bg-soft last:border-r-0 ${pref === value ? "bg-accent-soft text-accent" : ""}`}
              onClick={() => setPref(value as "" | "en" | "zh" | "zh-TW")}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}
