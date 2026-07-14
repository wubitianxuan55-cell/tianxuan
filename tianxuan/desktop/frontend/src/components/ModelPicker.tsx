import { useEffect, useRef, useState } from "react";
import { Check, ChevronsUpDown, Search, X } from "lucide-react";
import type { SettingsView } from "../lib/types";

interface ModelPickerOption {
  ref: string;
  provider: string;
  model: string;
  providerView?: { name: string; keySet: boolean; apiKeyEnv?: string };
}

function modelOptionFromRef(ref: string, s: SettingsView): ModelPickerOption | null {
  if (!ref) return null;
  const parts = ref.split("/");
  const provider = parts[0] || "";
  const model = parts.slice(1).join("/") || ref;
  const providerView = s.providers.find((p: any) => p.name === provider);
  return { ref, provider, model, providerView };
}

export function ModelPicker({
  s,
  refs: allRefs,
  value,
  disabled,
  ariaLabel,
  emptyOptionLabel,
  onPick,
}: {
  s: SettingsView;
  refs: string[];
  value: string;
  disabled: boolean;
  ariaLabel?: string;
  emptyOptionLabel?: string;
  onPick: (ref: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  // Close on click outside
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (
        panelRef.current && !panelRef.current.contains(e.target as Node) &&
        triggerRef.current && !triggerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") { setOpen(false); setQuery(""); }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [open]);

  const q = query.trim().toLowerCase();
  const selected = value ? modelOptionFromRef(value, s) : null;
  const selectedLabel = value === "" && emptyOptionLabel ? emptyOptionLabel : selected?.ref || value || emptyOptionLabel || "";

  // Build option list from allRefs
  const allOptions: ModelPickerOption[] = [];
  for (const ref of allRefs) {
    const parts = ref.split("/");
    const provider = parts[0] || "";
    const model = parts.slice(1).join("/") || ref;
    const providerView = s.providers.find((p: any) => p.name === provider);
    allOptions.push({ ref, provider, model, providerView });
  }

  // Filter
  const filtered = q
    ? allOptions.filter((opt) => opt.ref.toLowerCase().includes(q))
    : allOptions;

  // Group by provider
  const groups = new Map<string, ModelPickerOption[]>();
  for (const opt of filtered) {
    const g = groups.get(opt.provider) || [];
    g.push(opt);
    groups.set(opt.provider, g);
  }

  const emptyOptionVisible = emptyOptionLabel && (q === "" || (emptyOptionLabel.toLowerCase().includes(q)));

  return (
    <div className="relative">
      <button
        ref={triggerRef}
        type="button"
        className="flex items-center gap-1.5 w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 min-w-[140px] cursor-pointer hover:border-fg-faint transition-colors disabled:opacity-50"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        aria-label={ariaLabel}
        aria-expanded={open}
      >
        <span className="flex-1 truncate text-left">{selectedLabel}</span>
        <ChevronsUpDown size={12} className="text-fg-faint shrink-0" />
      </button>

      {open && (
        <div
          ref={panelRef}
          className="absolute z-50 top-full mt-1 left-0 right-0 bg-bg border border-border-soft rounded-lg shadow-lg overflow-hidden"
          style={{ minWidth: 240 }}
        >
          {/* Search */}
          <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border-soft">
            <Search size={13} className="text-fg-faint shrink-0" />
            <input
              className="flex-1 bg-transparent border-0 text-fg text-[12px] outline-none placeholder:text-fg-faint"
              placeholder="搜索模型..."
              value={query}
              autoFocus
              onChange={(e) => setQuery(e.target.value)}
            />
            {query && (
              <button
                type="button"
                className="bg-transparent border-0 text-fg-faint cursor-pointer p-0"
                onClick={() => setQuery("")}
              >
                <X size={13} />
              </button>
            )}
          </div>

          {/* Options */}
          <div className="max-h-64 overflow-y-auto">
            {emptyOptionVisible && (
              <button
                type="button"
                className={`w-full flex items-center gap-2 px-3 py-2 text-[12px] text-left cursor-pointer bg-transparent hover:bg-bg-soft border-0 ${
                  value === "" ? "bg-accent/10" : ""
                }`}
                onClick={() => { onPick(""); setOpen(false); setQuery(""); }}
              >
                <span className="flex-1 text-fg-faint">{emptyOptionLabel}</span>
                {value === "" && <Check size={13} className="text-accent" />}
              </button>
            )}

            {[...groups.entries()].map(([provider, options]) => (
              <div key={provider}>
                <div className="px-3 py-1.5 text-[11px] text-fg-faint font-semibold uppercase tracking-wide bg-bg-soft">
                  {provider}
                </div>
                {options.map((opt) => (
                  <button
                    key={opt.ref}
                    type="button"
                    className={`w-full flex items-center gap-2 px-3 py-2 text-[12px] text-left cursor-pointer bg-transparent hover:bg-bg-soft border-0 ${
                      opt.ref === value ? "bg-accent/10" : ""
                    }`}
                    onClick={() => { onPick(opt.ref); setOpen(false); setQuery(""); }}
                  >
                    <span className="flex-1 text-fg">{opt.model}</span>
                    {opt.ref === value && <Check size={13} className="text-accent" />}
                  </button>
                ))}
              </div>
            ))}

            {!emptyOptionVisible && groups.size === 0 && (
              <div className="px-3 py-4 text-[12px] text-fg-faint text-center">
                无匹配模型
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
