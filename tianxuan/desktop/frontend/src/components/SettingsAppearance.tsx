import { useState } from "react";
import { useI18n } from "../lib/i18n";
import type { ColorScheme, ThemeMode } from "../lib/theme";

const SCHEME_META: Record<ColorScheme, { accent: string; bg: string; keyword: string; str: string; num: string; info: string; label: string }> = {
  default: { accent: "#61AFEF", bg: "#282C34", keyword: "#C678DD", str: "#98C379", num: "#D19A66", info: "#56B6C2", label: "默认" },
  warm:    { accent: "#D79921", bg: "#282828", keyword: "#D3869B", str: "#B8BB26", num: "#FE8019", info: "#83A598", label: "暖色" },
  ice:     { accent: "#88C0D0", bg: "#2E3440", keyword: "#B48EAD", str: "#A3BE8C", num: "#D08770", info: "#81A1C1", label: "冰蓝" },
  forest:  { accent: "#83C092", bg: "#232A2E", keyword: "#D699B6", str: "#A7C080", num: "#E69875", info: "#7FBBB3", label: "森林" },
  sunset:  { accent: "#FFA759", bg: "#1F2430", keyword: "#D4BFFF", str: "#A8CC8C", num: "#FFA759", info: "#5CCFE6", label: "日落" },
  ocean:   { accent: "#89B4FA", bg: "#1E1E2E", keyword: "#CBA6F7", str: "#A6E3A1", num: "#FAB387", info: "#89DCEB", label: "海洋" },
  rose:    { accent: "#EB6F92", bg: "#1F1D2E", keyword: "#C4A7E7", str: "#9CCFD8", num: "#F6C177", info: "#C4A7E7", label: "玫瑰" },
  violet:  { accent: "#9D7CD8", bg: "#16161E", keyword: "#BB9AF7", str: "#9ECE6A", num: "#FF9E64", info: "#7DCFFF", label: "紫罗兰" },
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
                <div className="rounded-md mb-1.5 overflow-hidden" style={{ background: c.bg }}>
                  <div className="h-7" />
                  <div className="flex gap-px p-0.5 bg-black/30">
                    <span className="w-3 h-3 rounded-sm" style={{ background: c.accent }} />
                    <span className="w-3 h-3 rounded-sm" style={{ background: c.keyword }} />
                    <span className="w-3 h-3 rounded-sm" style={{ background: c.str }} />
                    <span className="w-3 h-3 rounded-sm" style={{ background: c.num }} />
                    <span className="w-3 h-3 rounded-sm" style={{ background: c.info }} />
                  </div>
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
