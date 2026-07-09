import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Brain, Check, ChevronsUpDown, Search } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { ModelInfo } from "../lib/types";
import { Tooltip } from "./Tooltip";

// ModelSwitcher is the model picker used in the header and settings. It opens a
// searchable dropdown grouped by provider with keyboard navigation.
// When allowInherit is true and the selected value is empty, the button shows
// inheritLabel and the dropdown includes an "inherit" option at the top.
export function ModelSwitcher({
  label,
  onPick,
  allowInherit = false,
  inheritLabel = "",
}: {
  label: string;
  onPick: (name: string) => void;
  allowInherit?: boolean;
  inheritLabel?: string;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [query, setQuery] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const loadModels = useCallback(() => {
    const p = app.Models();
    p.then(setModels).catch(() => {});
  }, []);

  useEffect(() => {
    if (open) {
      setQuery("");
      loadModels();
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open, loadModels]);

  const keyword = query.trim().toLowerCase();
  const filtered = useMemo(
    () =>
      keyword
        ? models.filter(
            (m) =>
              m.model.toLowerCase().includes(keyword) ||
              m.provider.toLowerCase().includes(keyword) ||
              m.ref.toLowerCase().includes(keyword),
          )
        : models,
    [models, keyword],
  );

  // Group by provider, with the current model's group first
  const groups = useMemo(() => {
    const map = new Map<string, ModelInfo[]>();
    let currentProvider = "";
    for (const m of filtered) {
      if (m.current) currentProvider = m.provider;
      const list = map.get(m.provider);
      if (list) list.push(m);
      else map.set(m.provider, [m]);
    }
    return [...map.entries()]
      .sort(([a], [b]) => {
        if (a === currentProvider) return -1;
        if (b === currentProvider) return 1;
        return providerLabel(a, t).localeCompare(providerLabel(b, t));
      })
      .map(([provider, items]) => ({
        provider,
        label: providerLabel(provider, t),
        items,
      }));
  }, [filtered, t]);

  const currentModel = models.find((m) => m.current) ?? models.find((m) => m.model === label || m.ref === label);
  const currentProvider = currentModel ? providerLabel(currentModel.provider, t) : null;
  const triggerLabel = currentProvider ? `${label} · ${currentProvider}` : label;

  const pick = useCallback(
    (name: string) => {
      setModels((prev) => prev.map((m) => ({ ...m, current: m.ref === name })));
      setOpen(false);
      onPick(name);
    },
    [onPick],
  );

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  return (
    <div className="modelsw" ref={containerRef}>
      <Tooltip label={triggerLabel}>
        <button
          type="button"
          className="modelsw__trigger"
          aria-label={triggerLabel}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          <Brain size={13} className="modelsw__kind" />
          <span className="modelsw__label">{label}</span>
          <ChevronsUpDown size={11} />
        </button>
      </Tooltip>
      {open && (
        <div
          className="modelsw__menu"
          role="listbox"
        >
          {/* search */}
          <div className="modelsw__search" role="presentation">
            <Search size={13} />
            <input
              ref={inputRef}
              type="text"
              className="modelsw__search-input"
              placeholder={t("modelSwitcher.searchPlaceholder")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") setOpen(false);
                if (e.key === "Enter" && filtered.length === 1) pick(filtered[0].ref);
              }}
            />
          </div>

          {models.length === 0 && (
            <div className="modelsw__empty">{t("status.noModels")}</div>
          )}
          {models.length > 0 && filtered.length === 0 && query && (
            <div className="modelsw__empty">{t("modelSwitcher.noMatches")}</div>
          )}

          {/* inherit option */}
          {allowInherit && (
            <button
              role="option"
              aria-selected={!label || label === inheritLabel}
              className={`modelsw__item ${!label || label === inheritLabel ? "modelsw__item--current" : ""}`}
              onClick={() => pick("")}
            >
              <span className="modelsw__model">{inheritLabel || t("settings.subagentInherit")}</span>
              {(!label || label === inheritLabel) && <Check size={13} className="modelsw__check" />}
            </button>
          )}

          {/* groups */}
          {groups.map((g) => (
            <div key={g.provider} role="group" aria-label={g.label} className="modelsw__group">
              <div className="modelsw__group-label" role="presentation">
                <Brain size={11} />
                {g.label}
              </div>
              {g.items.map((m) => (
                <button
                  key={m.ref}
                  type="button"
                  role="option"
                  aria-selected={m.current}
                  className={`modelsw__item ${m.current ? "modelsw__item--current" : ""}`}
                  onClick={() => pick(m.ref)}
                >
                  <span className="modelsw__model">{m.ref}</span>
                  {m.current && <Check size={13} className="modelsw__check" />}
                </button>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function providerLabel(provider: string, t: ReturnType<typeof useT>): string {
  switch (provider) {
    case "deepseek":
    case "deepseek-flash":
    case "deepseek-pro":
      return t("settings.providerLabel.deepseek");
    default:
      return provider;
  }
}
