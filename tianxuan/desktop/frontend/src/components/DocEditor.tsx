import { Pencil } from "lucide-react";
import { useT } from "../lib/i18n";
import { useState } from "react";
import type { MemoryDoc } from "../lib/types";

export function DocEditor(p: {
  docs: MemoryDoc[];
  onSaveDoc: (path: string, body: string) => Promise<void> | void;
  busy: boolean;
}) {
  const { docs, onSaveDoc, busy } = p;
  const t = useT();
  const [editingPath, setEditingPath] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

  return (
    <div className="flex flex-col gap-2">
      {docs.map((d) => {
        const editing = editingPath === d.path;
        return (
          <div className="border border-border-soft rounded-lg overflow-hidden" key={d.path}>
            <div className="flex items-center gap-2 px-2.5 py-1.5 bg-bg-soft/50">
              <span className="badge badge--project shrink-0">{d.scope}</span>
              <span
                className="flex-1 text-fg-dim font-mono text-[10.5px] truncate"
                title={d.path}
              >
                {d.path}
              </span>
              {!editing && (
                <button
                  className="px-2 py-0.5 text-[10.5px] border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
                  onClick={() => {
                    setEditingPath(d.path);
                    setDraft(d.body);
                  }}
                  type="button"
                >
                  <Pencil size={11} className="inline mr-1" />
                  {t("common.edit")}
                </button>
              )}
            </div>
            {editing ? (
              <div className="px-2.5 pb-2">
                <textarea
                  className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] p-2 outline-none resize-y min-h-[100px] focus:border-accent font-mono"
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  spellCheck={false}
                />
                <div className="flex justify-end gap-2 mt-1.5">
                  <button
                    className="px-2.5 py-1 text-[11px] border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
                    onClick={() => {
                      setEditingPath(null);
                      setDraft("");
                    }}
                    disabled={busy}
                    type="button"
                  >
                    {t("common.cancel")}
                  </button>
                  <button
                    className="px-2.5 py-1 text-[11px] border-0 rounded bg-accent text-accent-fg font-semibold cursor-pointer hover:brightness-110 active:scale-[0.97] transition-all disabled:opacity-40"
                    onClick={async () => {
                      const path = editingPath;
                      if (!path) return;
                      await onSaveDoc(path, draft);
                      setEditingPath(null);
                      setDraft("");
                    }}
                    disabled={busy}
                    type="button"
                  >
                    {t("common.save")}
                  </button>
                </div>
              </div>
            ) : (
              <pre className="m-0 px-3 py-2 bg-bg text-fg-dim text-[11px] leading-relaxed whitespace-pre-wrap border-t border-border-soft max-h-[160px] overflow-y-auto font-mono">
                {d.body}
              </pre>
            )}
          </div>
        );
      })}
    </div>
  );
}
