import { useState } from "react";
import { useI18n } from "../lib/i18n";
import type { ColorScheme, ThemeMode } from "../lib/theme";

const SCHEME_META: Record<ColorScheme, { accent: string; bg: string; label: string }> = {
  default: { accent: "#22C55E", bg: "#0F172A", label: "默认" },
  warm:    { accent: "#F59E0B", bg: "#1E1814", label: "暖色" },
  ice:     { accent: "#38BDF8", bg: "#0A111A", label: "冰蓝" },
  forest:  { accent: "#4ADE80", bg: "#0D1510", label: "森林" },
  sunset:  { accent: "#F97316", bg: "#1A1218", label: "日落" },
  ocean:   { accent: "#14B8A6", bg: "#0A1418", label: "海洋" },
  rose:    { accent: "#EC4899", bg: "#1A1018", label: "玫瑰" },
  violet:  { accent: "#A855F7", bg: "#14101A", label: "紫罗兰" },
};

const SCHEMES: ColorScheme[] = ["default", "warm", "ice", "forest", "sunset", "ocean", "rose", "violet"];

export function AppearanceSection({
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
  const { t, pref, setPref } = useI18n();

  // 字体偏好
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

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold mb-3">{t("settings.appearance")}</div>

      {/* ── 配色方案 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">配色方案</label>
        <div className="grid grid-cols-4 gap-2">
          {SCHEMES.map((s) => {
            const c = SCHEME_META[s];
            const isActive = scheme === s;
            return (
              <button
                key={s}
                onClick={() => onScheme(s)}
                className={`text-left bg-bg-soft border rounded-lg p-2 cursor-pointer transition-all hover:-translate-y-px hover:shadow-lg ${
                  isActive ? "border-accent ring-1 ring-accent/50" : "border-border-soft hover:border-fg-faint/30"
                }`}
              >
                <div className="flex items-center justify-center mb-1.5 rounded-md h-7" style={{ background: c.bg }}>
                  <span className="w-3 h-3 rounded-full" style={{ background: c.accent }} />
                </div>
                <span className={`text-[10px] font-medium block text-center ${isActive ? "text-accent" : "text-fg-dim"}`}>
                  {c.label}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      {/* ── 亮暗模式 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">亮暗模式</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {([
            { value: "light" as ThemeMode, icon: "☀️", label: "浅色" },
            { value: "dark" as ThemeMode,  icon: "🌙", label: "深色" },
            { value: "auto" as ThemeMode,  icon: "💻", label: "自动" },
          ]).map(({ value, icon, label }) => (
            <button
              key={value}
              className={`flex items-center gap-1.5 px-3.5 py-2 bg-transparent border-0 border-r border-border-soft text-fg-dim text-[13px] cursor-pointer transition-colors hover:text-fg hover:bg-bg-soft last:border-r-0 ${
                mode === value ? "bg-accent-soft text-accent" : ""
              }`}
              onClick={() => onMode(value)}
            >
              <span>{icon}</span>
              <span>{label}</span>
            </button>
          ))}
        </div>
      </div>

      {/* ── 界面字体 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">界面字体</label>
        <select className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={uiFont} onChange={e => applyFont("ui", e.target.value)}>
          <option value="">系统默认</option>
          <option value="pingfang">苹方 (PingFang SC)</option>
          <option value="yahei">微软雅黑</option>
          <option value="noto">Noto Sans SC</option>
        </select>
      </div>

      {/* ── 等宽字体 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">等宽字体</label>
        <select className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={monoFont} onChange={e => applyFont("mono", e.target.value)}>
          <option value="">系统默认</option>
          <option value="cascadia">Cascadia Code</option>
          <option value="jetbrains">JetBrains Mono</option>
          <option value="sfmono">SF Mono</option>
        </select>
      </div>

      {/* ── 语言 ── */}
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
