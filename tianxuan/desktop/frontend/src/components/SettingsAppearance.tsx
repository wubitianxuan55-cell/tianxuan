import { useState } from "react";
import { Sun, Moon, Monitor, Palette, Type } from "lucide-react";
import type { SectionProps } from "./SettingsShared";
import { SettingsPageShell, SettingsSection, SettingsField, SegmentedButton } from "./SettingsPageShell";
import { app } from "../lib/bridge";
import { applyColorScheme, applyThemeMode, getColorScheme, getThemeMode, type ColorScheme, type ThemeMode } from "../lib/theme";

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

function lsFallback<T>(key: string, deskVal: T, parse: (raw: string) => T): T {
  try { const raw = localStorage.getItem(key); if (raw !== null) return parse(raw); } catch { /* noop */ }
  return deskVal;
}

// ── FontChip — 字体预设胶囊按钮 ──
const UI_FONTS = [
  { value: "",       label: "系统默认" },
  { value: "pingfang", label: "苹方" },
  { value: "yahei",   label: "微软雅黑" },
  { value: "noto",    label: "Noto Sans" },
] as const;

const MONO_FONTS = [
  { value: "",         label: "系统默认" },
  { value: "cascadia", label: "Cascadia" },
  { value: "jetbrains",label: "JetBrains" },
  { value: "sfmono",   label: "SF Mono" },
] as const;

function FontChipGroup(p: {
  fonts: ReadonlyArray<{ value: string; label: string }>;
  current: string;
  known: string[];
  onChange: (v: string) => void;
}) {
  const isCurCustom = p.current !== "" && !p.known.includes(p.current);
  return (
    <div className="flex flex-wrap gap-1.5">
      {p.fonts.map((f) => {
        const active = !isCurCustom && f.value === p.current;
        return (
          <button
            key={f.value}
            type="button"
            className={`px-2.5 py-1 text-[11px] rounded border transition-colors ${
              active
                ? "border-accent bg-accent-soft text-accent font-semibold"
                : "border-border-soft bg-transparent text-fg-dim hover:border-fg-faint hover:text-fg"
            }`}
            onClick={() => p.onChange(f.value)}
          >{f.label}</button>
        );
      })}
      <button
        type="button"
        className={`px-2.5 py-1 text-[11px] rounded border transition-colors ${
          isCurCustom
            ? "border-accent bg-accent-soft text-accent font-semibold"
            : "border-border-soft bg-transparent text-fg-dim hover:border-fg-faint hover:text-fg"
        }`}
        onClick={() => p.onChange("custom")}
      >自定义…</button>
    </div>
  );
}

// ── main ──

export function AppearanceSection({ s, busy: _busy, apply }: SectionProps) {
  const desk = s.desktop;

  const [scheme, setSchemeState] = useState<ColorScheme>(() => getColorScheme());
  const [mode, setModeState] = useState<ThemeMode>(() => getThemeMode());

  const [uiFont, setUiFont] = useState(() => lsFallback("tianxuan.uiFont", desk.fontFamily || "", v => v));
  const [monoFont, setMonoFont] = useState(() => lsFallback("tianxuan.monoFont", desk.monoFontFamily || "", v => v));
  const [textSize, setTextSize] = useState(() => lsFallback("tianxuan.textSize", desk.textSize || "default", v => v));
  const [zoom, setZoom] = useState(() => lsFallback("tianxuan.zoom", desk.zoomFactor || 100, Number));

  const [uiCustomVal, setUiCustomVal] = useState("");
  const [monoCustomVal, setMonoCustomVal] = useState("");

  const KNOWN_UI = ["", "pingfang", "yahei", "noto"];
  const KNOWN_MONO = ["", "cascadia", "jetbrains", "sfmono"];

  const applyZoom = (z: number) => {
    document.documentElement.style.fontSize = `${z}%`;
    localStorage.setItem("tianxuan.zoom", String(z));
    void apply(() => app.SetDesktopZoomFactor(z));
  };

  const applyFont = (kind: "ui" | "mono", value: string) => {
    const attr = kind === "ui" ? "data-font-family" : "data-mono-font-family";
    const key = kind === "ui" ? "tianxuan.uiFont" : "tianxuan.monoFont";
    if (value && value !== "custom") {
      document.documentElement.setAttribute(attr, value);
      localStorage.setItem(key, value);
    } else {
      document.documentElement.removeAttribute(attr);
      localStorage.removeItem(key);
    }
    if (kind === "ui") {
      setUiFont(value);
      if (value !== "custom") void apply(() => app.SetDesktopFontFamily(value));
    } else {
      setMonoFont(value);
      if (value !== "custom") void apply(() => app.SetDesktopMonoFontFamily(value));
    }
  };

  const applyCustomFont = (kind: "ui" | "mono", value: string) => {
    if (!value.trim()) return;
    const attr = kind === "ui" ? "data-font-family" : "data-mono-font-family";
    const key = kind === "ui" ? "tianxuan.uiFont" : "tianxuan.monoFont";
    document.documentElement.setAttribute(attr, value.trim());
    localStorage.setItem(key, value.trim());
    if (kind === "ui") {
      setUiFont(value.trim());
      void apply(() => app.SetDesktopFontFamily(value.trim()));
    } else {
      setMonoFont(value.trim());
      void apply(() => app.SetDesktopMonoFontFamily(value.trim()));
    }
  };

  const updateScheme = (sc: ColorScheme) => { applyColorScheme(sc); setSchemeState(sc); void apply(() => app.SetDesktopThemeStyle(sc)); };
  const updateMode = (m: ThemeMode) => { applyThemeMode(m); setModeState(m); void apply(() => app.SetDesktopTheme(m)); };
  const updateTextSize = (v: string) => {
    localStorage.setItem("tianxuan.textSize", v);
    document.documentElement.setAttribute("data-text-size", v);
    setTextSize(v);
    void apply(() => app.SetDesktopTextSize(v));
  };

  return (
    <SettingsPageShell title="外观" desc="配色方案、字体与显示缩放。">
      {/* ── 配色方案 ── */}
      <SettingsSection title={<span className="flex items-center gap-1.5"><Palette size={14} className="text-fg-faint" />配色方案</span>}>
        <SettingsField label="主题风格" hint="选择界面主色调。">
          <div className="grid grid-cols-4 gap-2">
            {SCHEMES.map((s) => {
              const c = SCHEME_META[s];
              const isActive = scheme === s;
              return (
                <button
                  key={s}
                  onClick={() => updateScheme(s)}
                  className={`text-left bg-bg-soft border rounded-lg p-2 cursor-pointer transition-all hover:-translate-y-px hover:shadow-lg ${
                    isActive ? "border-accent ring-1 ring-accent/50" : "border-border-soft hover:border-fg-faint/30"
                  }`}
                >
                  <div className="rounded-md mb-1.5 overflow-hidden" style={{ background: `linear-gradient(135deg, ${c.bg} 0%, ${c.accent}40 100%)` }}>
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
        </SettingsField>

        <SettingsField label="亮暗模式" hint="浅色、深色或跟随系统自动切换。">
          <SegmentedButton
            options={[
              { value: "light" as ThemeMode, label: <span className="flex items-center gap-1"><Sun size={13} />浅色</span> as any },
              { value: "dark" as ThemeMode,  label: <span className="flex items-center gap-1"><Moon size={13} />深色</span> as any },
              { value: "auto" as ThemeMode,  label: <span className="flex items-center gap-1"><Monitor size={13} />自动</span> as any },
            ]}
            value={mode}
            onChange={updateMode}
          />
        </SettingsField>
      </SettingsSection>

      {/* ── 字体与缩放 ── */}
      <SettingsSection title={<span className="flex items-center gap-1.5"><Type size={14} className="text-fg-faint" />字体与缩放</span>}>
        <SettingsField label="界面字体" hint="整体界面的显示字体。">
          <FontChipGroup fonts={UI_FONTS} current={uiFont} known={KNOWN_UI} onChange={(v) => {
            if (v === "custom") setUiCustomVal("");
            else applyFont("ui", v);
          }} />
          {(uiCustomVal !== undefined && (uiFont === "custom" || (uiFont !== "" && !KNOWN_UI.includes(uiFont) && uiFont !== "custom")) && (
            <div className="flex gap-1.5 mt-1.5">
              <input
                className="flex-1 bg-bg border border-border rounded-md text-fg text-[12px] px-2 py-1 outline-none focus:border-accent"
                placeholder="输入字体名称…"
                defaultValue={!KNOWN_UI.includes(uiFont) && uiFont !== "custom" ? uiFont : ""}
                onChange={e => setUiCustomVal(e.target.value)}
              />
              <button
                className="px-2.5 py-1 text-[11px] rounded bg-accent text-white border-0 cursor-pointer font-semibold"
                onClick={() => applyCustomFont("ui", uiCustomVal)}
              >应用</button>
            </div>
          ))}
        </SettingsField>

        <SettingsField label="等宽字体" hint="代码区域的等宽字体。">
          <FontChipGroup fonts={MONO_FONTS} current={monoFont} known={KNOWN_MONO} onChange={(v) => {
            if (v === "custom") setMonoCustomVal("");
            else applyFont("mono", v);
          }} />
          {(monoCustomVal !== undefined && (monoFont === "custom" || (monoFont !== "" && !KNOWN_MONO.includes(monoFont) && monoFont !== "custom"))) && (
            <div className="flex gap-1.5 mt-1.5">
              <input
                className="flex-1 bg-bg border border-border rounded-md text-fg text-[12px] px-2 py-1 outline-none focus:border-accent"
                placeholder="输入字体名称…"
                defaultValue={monoFont !== "" && !KNOWN_MONO.includes(monoFont) && monoFont !== "custom" ? monoFont : ""}
                onChange={e => setMonoCustomVal(e.target.value)}
              />
              <button
                className="px-2.5 py-1 text-[11px] rounded bg-accent text-white border-0 cursor-pointer font-semibold"
                onClick={() => applyCustomFont("mono", monoCustomVal)}
              >应用</button>
            </div>
          )}
        </SettingsField>

        <SettingsField label="字体大小" hint="聊天内容的字号。">
          <SegmentedButton
            options={[
              { value: "small", label: "小" },
              { value: "default", label: "默认" },
              { value: "large", label: "大" },
              { value: "xlarge", label: "加大" },
            ]}
            value={textSize}
            onChange={updateTextSize}
          />
        </SettingsField>

        <SettingsField label="显示缩放" hint="整体缩放比例 (70%–150%)。">
          <div className="flex items-center gap-2">
            <button
              className="w-7 h-7 rounded border border-border-soft bg-transparent text-fg-dim text-sm cursor-pointer hover:bg-bg-soft hover:text-fg flex items-center justify-center transition-colors"
              onClick={() => { const n = Math.max(70, zoom - 10); setZoom(n); applyZoom(n); }}
            >−</button>
            <input
              type="range" min="70" max="150" step="5" value={zoom}
              onChange={(e) => { const v = Number(e.target.value); setZoom(v); applyZoom(v); }}
              className="w-[120px] accent-accent h-1.5"
            />
            <button
              className="w-7 h-7 rounded border border-border-soft bg-transparent text-fg-dim text-sm cursor-pointer hover:bg-bg-soft hover:text-fg flex items-center justify-center transition-colors"
              onClick={() => { const n = Math.min(150, zoom + 10); setZoom(n); applyZoom(n); }}
            >+</button>
            <span className="text-[12px] text-fg-dim font-mono min-w-[42px] tabular-nums">{zoom}%</span>
            <button
              className="px-2 py-1 text-[11px] rounded border border-border-soft bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
              onClick={() => { setZoom(100); applyZoom(100); }}
            >重置</button>
          </div>
        </SettingsField>
      </SettingsSection>
    </SettingsPageShell>
  );
}
