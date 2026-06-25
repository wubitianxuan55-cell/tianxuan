import { memo, useEffect, useState } from "react";
import { Check, ChevronsUpDown } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { ModelInfo } from "../lib/types";

// ModelSwitcher is the header model picker: the model label becomes a button
// that opens a dropdown listing configured providers.
export const ModelSwitcher = memo(function ModelSwitcher({ label, onPick }: { label: string; onPick: (name: string) => void }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [models, setModels] = useState<ModelInfo[]>([]);

  useEffect(() => {
    if (open) app.Models().then(setModels).catch(() => {});
  }, [open]);

  const pick = (name: string) => {
    setOpen(false);
    onPick(name);
  };

  return (
    <div className="relative inline-flex">
      <button className="flex items-center gap-1 px-1.5 py-0.5 border border-border-soft rounded-lg bg-transparent text-fg-dim text-[12px] font-medium cursor-pointer no-drag hover:text-fg hover:border-fg-faint" onClick={() => setOpen((v) => !v)} title={t("status.switchModel")}>
        <span className="max-w-28 truncate font-mono text-[11px]">{label}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} />
          <div className="absolute top-full left-1/2 -translate-x-1/2 mt-1 w-60 max-h-64 overflow-y-auto bg-bg-elev-2 border border-border rounded-lg z-20 p-1" role="listbox" style={{boxShadow: "var(--ds-shadow-dropdown)"}}>
            {models.length === 0 && <div className="px-3 py-4 text-fg-faint text-xs text-center">{t("status.noModels")}</div>}
            {models.map((m) => (
              <button
                key={m.ref}
                role="option"
                aria-selected={m.current}
                className={`flex items-center gap-2.5 w-full px-2.5 py-2 bg-transparent border-0 rounded-md text-left cursor-pointer text-fg-dim text-[13px] hover:bg-bg-soft hover:text-fg ${m.current ? "text-accent bg-accent-soft font-semibold hover:bg-accent-soft hover:text-accent" : ""}`}
                onClick={() => pick(m.ref)}
              >
                <span className="flex-1 min-w-0 text-left font-medium">{m.model}</span>
                {m.current && <Check size={13} className="shrink-0 text-accent" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
});
