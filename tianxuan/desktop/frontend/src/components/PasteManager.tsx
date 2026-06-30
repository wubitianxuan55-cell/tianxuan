import { useCallback, useRef, useState } from "react";
import type { ClipboardEvent, RefObject } from "react";
import { Eye, FileText, Trash2 } from "lucide-react";
import { useT } from "../lib/i18n";

// ── 类型与常量 ──

export interface PastedBlock { label: string; text: string }

export const LONG_PASTE_MIN_CHARS = 2000;
export const LONG_PASTE_MIN_LINES = 20;

export function lineCount(s: string): number {
  if (s === "") return 0;
  return s.split(/\r\n|\r|\n/).length;
}

export function shouldFoldPaste(s: string): boolean {
  return s.length >= LONG_PASTE_MIN_CHARS || lineCount(s) >= LONG_PASTE_MIN_LINES;
}

export function renderPastedBlock(block: PastedBlock): string {
  return `${block.label}\n\n--- Begin ${block.label} ---\n${block.text}\n--- End ${block.label} ---`;
}

// ── Hook ──

export function usePasteBlocks(
  text: string,
  setText: (t: string) => void,
  textareaRef: RefObject<HTMLTextAreaElement>,
) {
  const t = useT();
  const [pastedBlocks, setPastedBlocks] = useState<PastedBlock[]>([]);
  const [openPastedLabels, setOpenPastedLabels] = useState<string[]>([]);
  const [pendingPaste, setPendingPaste] = useState(0);
  const pastedBlocksRef = useRef<PastedBlock[]>([]);
  const nextPasteId = useRef(1);

  const activePastedBlocks = pastedBlocks.filter((b) => text.includes(b.label));

  const onPaste = useCallback((e: ClipboardEvent<HTMLTextAreaElement>) => {
    const pasted = e.clipboardData.getData("text");
    if (!shouldFoldPaste(pasted)) return;
    e.preventDefault();
    const ta = e.currentTarget;
    const start = ta.selectionStart ?? text.length;
    const end = ta.selectionEnd ?? text.length;
    const id = nextPasteId.current++;
    const label = t("composer.pastedLabel", { id, lines: lineCount(pasted) });
    const block: PastedBlock = { label, text: pasted };
    const next = text.slice(0, start) + label + text.slice(end);
    pastedBlocksRef.current = [...pastedBlocksRef.current, block];
    setPastedBlocks((prev) => [...prev, block]);
    setText(next);
    requestAnimationFrame(() => {
      const n = textareaRef.current;
      if (n) { n.focus(); n.selectionStart = n.selectionEnd = start + label.length; }
    });
  }, [text, setText, textareaRef, t]);

  const expandBlocks = useCallback((displayText: string): string => {
    let expanded = displayText;
    for (const b of pastedBlocksRef.current) {
      if (expanded.includes(b.label)) expanded = expanded.split(b.label).join(renderPastedBlock(b));
    }
    return expanded;
  }, []);

  const togglePreview = useCallback((label: string) => {
    setOpenPastedLabels((p) => p.includes(label) ? p.filter((x) => x !== label) : [...p, label]);
  }, []);

  const removeBlock = useCallback((b: PastedBlock) => {
    pastedBlocksRef.current = pastedBlocksRef.current.filter((x) => x.label !== b.label);
    setPastedBlocks((p) => p.filter((x) => x.label !== b.label));
    setOpenPastedLabels((p) => p.filter((x) => x !== b.label));
    setText(text.split(b.label).join(""));
  }, [text, setText]);

  const expandBlock = useCallback((b: PastedBlock) => {
    pastedBlocksRef.current = pastedBlocksRef.current.filter((x) => x.label !== b.label);
    setPastedBlocks((p) => p.filter((x) => x.label !== b.label));
    setOpenPastedLabels((p) => p.filter((x) => x !== b.label));
    setText(text.split(b.label).join(b.text));
  }, [text, setText]);

  const clearBlocks = useCallback(() => {
    pastedBlocksRef.current = [];
    setPastedBlocks([]);
    setOpenPastedLabels([]);
  }, []);

  const setPendingPasteCount = useCallback((n: number) => {
    setPendingPaste((prev) => Math.max(0, prev + n));
  }, []);

  return {
    pastedBlocks,
    openPastedLabels,
    pendingPaste,
    activePastedBlocks,
    onPaste,
    expandBlocks,
    togglePreview,
    removeBlock,
    expandBlock,
    clearBlocks,
    setPendingPasteCount,
  };
}

// ── UI 组件 ──

export function PasteBlocksUI({
  blocks,
  openLabels,
  onTogglePreview,
  onExpand,
  onRemove,
}: {
  blocks: PastedBlock[];
  openLabels: string[];
  onTogglePreview: (label: string) => void;
  onExpand: (b: PastedBlock) => void;
  onRemove: (b: PastedBlock) => void;
}) {
  if (blocks.length === 0) return null;
  return (
    <div className="pb-1.5">
      {blocks.map((block) => {
        const open = openLabels.includes(block.label);
        return (
          <div className="mb-1 border border-border-soft rounded-lg overflow-hidden" key={block.label}>
            <div className="flex items-center gap-1.5 px-2 py-1 text-xs">
              <FileText size={14} className="text-fg-faint shrink-0" />
              <span className="font-mono text-xs text-fg-dim min-w-0 truncate">{block.label}</span>
              <div className="flex items-center gap-0.5 ml-auto">
                <ActionBtn title={open ? "收起预览" : "展开预览"} onClick={() => onTogglePreview(block.label)}>
                  <Eye size={13} />
                </ActionBtn>
                <ActionBtn title="展开内容" onClick={() => onExpand(block)}>
                  <span className="text-[11px]">展开</span>
                </ActionBtn>
                <ActionBtn title="移除" danger onClick={() => onRemove(block)}>
                  <Trash2 size={13} />
                </ActionBtn>
              </div>
            </div>
            {open && (
              <pre className="m-0 p-2 bg-bg text-fg-dim text-xs leading-relaxed whitespace-pre-wrap break-words max-h-[140px] overflow-y-auto border-t border-border-soft">
                {block.text}
              </pre>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ActionBtn({ title, danger, onClick, children }: { title: string; danger?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" className={`px-1.5 py-0.5 bg-transparent border-0 rounded text-fg-faint cursor-pointer text-[11px] transition-colors ${danger ? "hover:text-err hover:bg-bg-soft" : "hover:text-fg hover:bg-bg-soft"}`} title={title} onClick={onClick}>
      {children}
    </button>
  );
}
