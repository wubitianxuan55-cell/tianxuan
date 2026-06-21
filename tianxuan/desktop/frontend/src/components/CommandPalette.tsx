import { useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Command, Search } from "lucide-react";
import { useMountTransition } from "../lib/useMountTransition";
import { useT } from "../lib/i18n";

// CommandPalette is a Ctrl+K modal that surfaces commands, sessions, and
// recent files via fuzzy search. Items are provided by the caller (App) so
// the palette stays decoupled from the controller.
//
// Interaction model:
//   - Input is auto-focused on open; the first match is highlighted.
//   - ↑/↓ move the highlight (wraps at the edges).
//   - Enter runs the highlighted item's action.
//   - Esc closes.
export interface PaletteItem {
  id: string;
  title: string;
  hint?: string;
  meta?: string;
  badge?: string;
  icon?: ReactNode;
  compact?: boolean;
  group: string;
  keywords?: string[];
  run: () => void | Promise<void>;
}

// Fuzzy match: every query token must appear in the candidate's title or hint,
// in order (case-insensitive).
function fuzzyMatch(query: string, item: PaletteItem): boolean {
  if (!query.trim()) return true;
  const tokens = query.trim().toLowerCase().split(/\s+/);
  const haystack = [
    item.title,
    item.hint ?? "",
    ...(item.keywords ?? []),
  ]
    .join(" ")
    .toLowerCase();
  let pos = 0;
  for (const token of tokens) {
    const idx = haystack.indexOf(token, pos);
    if (idx < 0) return false;
    pos = idx + token.length;
  }
  return true;
}

export function CommandPalette({
  open,
  items,
  onClose,
}: {
  open: boolean;
  items: PaletteItem[];
  onClose: () => void;
}) {
  const t = useT();
  const { mounted, status } = useMountTransition(open, 180);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // Reset query + active on open
  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
      // Auto-focus: defer to after the mount animation frame
      const raf = requestAnimationFrame(() => inputRef.current?.focus());
      return () => cancelAnimationFrame(raf);
    }
  }, [open]);

  // Filter + group
  const groups = useMemo(() => {
    const filtered = query.trim()
      ? items.filter((it) => fuzzyMatch(query, it))
      : items;
    const map = new Map<string, PaletteItem[]>();
    for (const it of filtered) {
      const arr = map.get(it.group) ?? [];
      arr.push(it);
      map.set(it.group, arr);
    }
    return Array.from(map.entries()).map(([name, its]) => ({
      name,
      items: its,
    }));
  }, [items, query]);

  const flat = useMemo(
    () => groups.flatMap((g) => g.items),
    [groups],
  );

  // Clamp active
  const safeActive = flat.length === 0 ? 0 : Math.min(active, flat.length - 1);

  // Scroll active into view
  useEffect(() => {
    if (flat.length === 0) return;
    const el = listRef.current?.querySelector(`[data-palette-active="true"]`);
    el?.scrollIntoView({ block: "nearest" });
  }, [safeActive, flat.length]);

  // Keyboard
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => (a + 1) % Math.max(flat.length, 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => (a - 1 + flat.length) % Math.max(flat.length, 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (flat[safeActive]) {
        void flat[safeActive].run();
        onClose();
      }
    } else if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    }
  };

  if (!mounted) return null;

  const empty = flat.length === 0 && query.trim() !== "";

  return (
    <div
      className="drawer-backdrop"
      style={{
        justifyContent: "center",
        alignItems: "flex-start",
        paddingTop: "12vh",
        background: "var(--ds-bg-app)",
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="palette" data-state={status}>
        {/* Search input */}
        <div className="palette__inputrow">
          <Search size={17} className="palette__search-icon" color="var(--fg-faint)" />
          <input
            ref={inputRef}
            className="palette__input"
            type="text"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActive(0);
            }}
            onKeyDown={onKeyDown}
            placeholder={t("palette.placeholder") ?? "搜索命令、会话…"}
            spellCheck={false}
          />
          <span className="palette__esc">esc</span>
        </div>

        {/* Results */}
        <div className="palette__list" ref={listRef}>
          {empty ? (
            <div className="palette__empty">{t("palette.empty") ?? "无匹配结果"}</div>
          ) : (
            groups.map((g) => {
              return (
                <div key={g.name} className="palette__group">
                  <div className="palette__group-title">{g.name}</div>
                  {g.items.every((it) => it.compact) ? (
                    <div className="palette__grid">
                      {g.items.map((it) => {
                        const idx = flat.indexOf(it);
                        const on = idx === safeActive;
                        return (
                          <button
                            type="button"
                            key={it.id}
                            className={`palette__chip${on ? " palette__chip--on" : ""}`}
                            onMouseEnter={() => setActive(flat.indexOf(it))}
                            onClick={() => { void it.run(); onClose(); }}
                          >
                            <span className="palette__chip-icon">{it.icon ?? <Command size={15} />}</span>
                            <span className="palette__chip-label">{it.title}</span>
                          </button>
                        );
                      })}
                    </div>
                  ) : (
                    g.items.map((it) => {
                      const idx = flat.indexOf(it);
                      const on = idx === safeActive;
                      return (
                        <button
                          type="button"
                          role="option"
                          aria-selected={on}
                          key={it.id}
                          className={`palette__item${on ? " palette__item--on" : ""}`}
                          data-palette-active={on}
                          onMouseEnter={() => setActive(idx)}
                          onClick={() => { void it.run(); onClose(); }}
                        >
                          <span className="palette__item-icon" aria-hidden="true">
                            {it.icon ?? <Command size={15} />}
                          </span>
                          <span className="palette__body">
                            <span className="palette__title">{it.title}</span>
                            {(it.hint || it.meta || it.badge) && (
                              <span className="palette__hint">
                                {it.hint && <span className="palette__hint-text">{it.hint}</span>}
                                {it.meta && <span className="palette__meta">{it.meta}</span>}
                                {it.badge && <span className="palette__badge">{it.badge}</span>}
                              </span>
                            )}
                          </span>
                        </button>
                      );
                    })
                  )}
                </div>
              );
            })
          )}
        </div>

        {/* Footer */}
        <div className="palette__foot">
          <span><kbd>↑</kbd><kbd>↓</kbd> 导航</span>
          <span><kbd>↵</kbd> 选择</span>
          <span><kbd>esc</kbd> 关闭</span>
        </div>
      </div>
    </div>
  );
}
