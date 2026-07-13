import { useState } from "react";
import { useI18n } from "../lib/i18n";
import type { ColorScheme, ThemeMode } from "../lib/theme";

const SCHEME_META: Record<ColorScheme, { accent: string; bg: string; keyword: string; str: string; num: string; info: string; label: string }> = {
  default: { accent: "#22C55E", bg: "#020617", keyword: "#C084FC", str: "#4ADE80", num: "#FB923C", info: "#38BDF8", label: "默认" },
  warm:    { accent: "#F59E0B", bg: "#1E1814", keyword: "#C084FC", str: "#4ADE80", num: "#FB923C", info: "#2DD4BF", label: "暖色" },
  ice:     { accent: "#38BDF8", bg: "#050510", keyword: "#A78BFA", str: "#22C55E", num: "#FB923C", info: "#38BDF8", label: "冰蓝" },
  forest:  { accent: "#4ADE80", bg: "#0A120C", keyword: "#C084FC", str: "#4ADE80", num: "#FB923C", info: "#2DD4BF", label: "森林" },
  sunset:  { accent: "#F43F5E", bg: "#0F0F23", keyword: "#A78BFA", str: "#4ADE80", num: "#FBBF24", info: "#38BDF8", label: "霓虹" },
  ocean:   { accent: "#818CF8", bg: "#0A0A1C", keyword: "#A78BFA", str: "#4ADE80", num: "#FB923C", info: "#38BDF8", label: "午夜" },
  rose:    { accent: "#EC4899", bg: "#1A1018", keyword: "#C084FC", str: "#4ADE80", num: "#FB923C", info: "#38BDF8", label: "玫红" },
  violet:  { accent: "#3B82F6", bg: "#09090B", keyword: "#D4D4D8", str: "#A1A1AA", num: "#E4E4E7", info: "#38BDF8", label: "石墨" },
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

  const [uiFont, setUiFont] = useState(() => {
    try { return localStorage.getItem("tianxuan.uiFont") || ""; } catch { return ""; }
  });
  const [monoFont, setMonoFont] = useState(() => {
    try { return localStorage.getItem("tianxuan.monoFont") || ""; } catch { return ""; }
  });
  const [textSize, setTextSize] = useState(() => {
    try { return localStorage.getItem("tianxuan.textSize") || "default"; } catch { return "default"; }
  });
  const [zoom, setZoom] = useState(() => {
    try { return Number(localStorage.getItem("tianxuan.zoom")) || 100; } catch { return 100; }
  });
  const [layout, setLayout] = useState(() => {
    try { return localStorage.getItem("tianxuan.layoutStyle") || "classic"; } catch { return "classic"; }
  });
  const [close, setClose] = useState(() => {
    try { return localStorage.getItem("tianxuan.closeBehavior") || "quit"; } catch { return "quit"; }
  });
  const applyZoom = (z: number) => {
    localStorage.setItem("tianxuan.zoom", String(z));
    document.documentElement.style.fontSize = `${z}%`;
  };
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

      {/* ── 字体大小 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">字体大小</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {[
            { value: "small", label: "小" },
            { value: "default", label: "默认" },
            { value: "large", label: "大" },
            { value: "xlarge", label: "加大" },
          ].map(({ value, label }) => (
            <button
              key={value}
              className={`px-3 py-1.5 bg-transparent border-0 border-r border-border-soft text-fg-dim text-xs cursor-pointer transition-[color,background] hover:text-fg hover:bg-bg-soft last:border-r-0 ${textSize === value ? "bg-accent-soft text-accent" : ""}`}
              onClick={() => {
                localStorage.setItem("tianxuan.textSize", value);
                document.documentElement.setAttribute("data-text-size", value);
                setTextSize(value);
              }}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* ── 显示缩放 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">显示缩放</label>
        <div className="flex items-center gap-2">
          <button className="w-6 h-6 rounded border border-border-soft bg-transparent text-fg-dim text-xs cursor-pointer hover:bg-bg-soft flex items-center justify-center"
            onClick={() => { const n = Math.max(70, zoom - 10); setZoom(n); applyZoom(n); }}>−</button>
          <input type="range" min="70" max="150" step="5" value={zoom}
            onChange={(e) => { const v = Number(e.target.value); setZoom(v); applyZoom(v); }}
            className="w-[120px] accent-accent h-1" />
          <button className="w-6 h-6 rounded border border-border-soft bg-transparent text-fg-dim text-xs cursor-pointer hover:bg-bg-soft flex items-center justify-center"
            onClick={() => { const n = Math.min(150, zoom + 10); setZoom(n); applyZoom(n); }}>+</button>
          <span className="text-[11px] text-fg-faint min-w-[36px]">{zoom}%</span>
          <button className="px-2 py-1 text-[11px] rounded border border-border-soft bg-transparent text-fg-dim cursor-pointer hover:bg-bg-soft"
            onClick={() => { setZoom(100); applyZoom(100); }}>重置</button>
        </div>
      </div>

      {/* ── 桌面布局 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">布局风格</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {[
            { value: "classic", label: "经典" },
            { value: "workbench", label: "工作台" },
            { value: "creation", label: "创作" },
          ].map(({ value, label }) => (
            <button
              key={value}
              className={`px-3 py-1.5 bg-transparent border-0 border-r border-border-soft text-fg-dim text-xs cursor-pointer transition-[color,background] hover:text-fg hover:bg-bg-soft last:border-r-0 ${layout === value ? "bg-accent-soft text-accent" : ""}`}
              onClick={() => {
                localStorage.setItem("tianxuan.layoutStyle", value);
                document.documentElement.setAttribute("data-layout-style", value);
                setLayout(value);
              }}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* ── 关闭行为 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">关闭行为</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {[
            { value: "quit", label: "退出" },
            { value: "background", label: "最小化到托盘" },
          ].map(({ value, label }) => (
            <button
              key={value}
              className={`px-3 py-1.5 bg-transparent border-0 border-r border-border-soft text-fg-dim text-xs cursor-pointer transition-[color,background] hover:text-fg hover:bg-bg-soft last:border-r-0 ${close === value ? "bg-accent-soft text-accent" : ""}`}
              onClick={() => { localStorage.setItem("tianxuan.closeBehavior", value); setClose(value); }}
            >
              {label}
            </button>
          ))}
        </div>
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
