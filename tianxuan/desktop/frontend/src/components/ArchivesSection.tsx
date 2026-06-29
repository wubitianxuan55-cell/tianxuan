import { memo, useState } from "react";
import { useT } from "../lib/i18n";

type ArchivedItem = {
  name: string;
  title?: string;
  description: string;
  type: string;
  path?: string;
  archivedAt?: string;
};

export const ArchivesSection = memo(function ArchivesSection(p: {
  archives: ArchivedItem[];
}) {
  const st = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        className="flex items-center gap-1.5 text-fg-faint text-[11px] font-semibold uppercase tracking-wider bg-transparent border-0 cursor-pointer hover:text-fg transition-colors"
        onClick={() => setOpen((v) => !v)}
        type="button"
        aria-expanded={open}
      >
        {open ? "\u25BE" : "\u25B8"} {st("memory.archived")}
        <span className="text-fg-faint/60 font-normal">({p.archives.length})</span>
      </button>
      {open && (
        <div className="mt-2 flex flex-col gap-1.5">
          {p.archives.map((a) => (
            <div
              key={a.name}
              className="border border-border-soft rounded-lg px-3 py-2 bg-bg-soft/50 opacity-70 hover:opacity-100 transition-opacity"
            >
              <div className="flex items-center gap-2">
                <span className="text-fg-dim text-[12px] font-medium">{a.title || a.name}</span>
                <span className="badge badge--muted">{a.type}</span>
                {a.archivedAt && (
                  <span className="text-fg-faint text-[10px] ml-auto font-mono">
                    {new Date(a.archivedAt).toLocaleDateString()}
                  </span>
                )}
              </div>
              {a.description && (
                <div className="text-fg-faint text-[10.5px] mt-0.5">{a.description}</div>
              )}
            </div>
          ))}
        </div>
      )}
    </>
  );
});
