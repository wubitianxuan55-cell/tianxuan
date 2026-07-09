// theme.ts — 双属性主题系统：data-theme-scheme（配色）+ data-theme-mode（亮暗）
// 旧 data-theme 协议通过 migrateLegacyTheme() 一次性迁移后删除。

export type ColorScheme = "default" | "warm" | "ice" | "forest" | "sunset" | "ocean" | "rose" | "violet";
export type ThemeMode = "light" | "dark" | "auto";

const SCHEME_KEY = "tianxuan-color-scheme";
const MODE_KEY = "tianxuan-theme-mode";
const LEGACY_KEY = "tianxuan-theme";

// ── 旧协议迁移 ────────────────────────────────────────────────────────
function migrateLegacyTheme(): void {
  try {
    const raw = localStorage.getItem(LEGACY_KEY);
    if (raw === null) return;
    localStorage.removeItem(LEGACY_KEY);
    let v = raw;
    try {
      const p = JSON.parse(raw);
      if (typeof p === "string") v = p;
      else if (p && typeof p === "object" && typeof (p as any).mode === "string") v = (p as any).mode;
    } catch { /* raw string */ }

    let scheme: ColorScheme = "default";
    let mode: ThemeMode = "dark";

    switch (v) {
      case "auto":
        scheme = "default"; mode = "auto"; break;
      case "light":
      case "focus":
        scheme = "default"; mode = "light"; break;
      case "dark":
      case "contrast":
      case "midnight":
      case "neon":
      case "mono":
        scheme = "default"; mode = "dark"; break;
      case "warm":
        scheme = "warm"; mode = "dark"; break;
      case "ice":
        scheme = "ice"; mode = "dark"; break;
      case "forest":
        scheme = "forest"; mode = "dark"; break;
      default:
        return; // 不认识的值不迁移
    }

    localStorage.setItem(SCHEME_KEY, scheme);
    localStorage.setItem(MODE_KEY, mode);
  } catch { /* 无痕模式 */ }
}

// ── 验证 ──────────────────────────────────────────────────────────────
function validateScheme(v: unknown): ColorScheme {
  const schemes: ColorScheme[] = ["default", "warm", "ice", "forest", "sunset", "ocean", "rose", "violet"];
  if (typeof v === "string" && (schemes as string[]).includes(v)) return v as ColorScheme;
  return "default";
}

function validateMode(v: unknown): ThemeMode {
  if (v === "light" || v === "dark" || v === "auto") return v;
  return "dark";
}

// ── 读写 ──────────────────────────────────────────────────────────────
export function getColorScheme(): ColorScheme {
  migrateLegacyTheme();
  try { return validateScheme(localStorage.getItem(SCHEME_KEY)); } catch { return "default"; }
}

export function getThemeMode(): ThemeMode {
  migrateLegacyTheme();
  try { return validateMode(localStorage.getItem(MODE_KEY)); } catch { return "dark"; }
}

// ── DOM 操作 ──────────────────────────────────────────────────────────
export function applyColorScheme(scheme: ColorScheme): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  root.setAttribute("data-theme-scheme", scheme);
  try { localStorage.setItem(SCHEME_KEY, scheme); } catch { /* 无痕 */ }
}

export function applyThemeMode(mode: ThemeMode): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  if (mode === "auto") root.removeAttribute("data-theme-mode");
  else root.setAttribute("data-theme-mode", mode);
  try { localStorage.setItem(MODE_KEY, mode); } catch { /* 无痕 */ }
}

// ── 初始化 ────────────────────────────────────────────────────────────
export function initTheme(): void {
  migrateLegacyTheme();
  const scheme = getColorScheme();
  const mode = getThemeMode();
  if (typeof document !== "undefined") {
    const root = document.documentElement;
    root.setAttribute("data-theme-scheme", scheme);
    if (mode === "auto") root.removeAttribute("data-theme-mode");
    else root.setAttribute("data-theme-mode", mode);
  }
}

// ── 向后兼容（供过渡期消费方引用）──────────────────────────────────────
/** @deprecated 用 ColorScheme + ThemeMode 替代 */
export type Theme = "auto" | "light" | "dark" | "warm" | "ice" | "forest";
