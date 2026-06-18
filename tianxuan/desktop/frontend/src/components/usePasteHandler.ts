import { useRef, useState } from "react";
import { useT } from "../lib/i18n";

const LONG_PASTE_MIN_CHARS = 2000;
const LONG_PASTE_MIN_LINES = 20;

export type PastedBlock = {
  label: string;
  text: string;
};

function lineCount(s: string): number {
  if (s === "") return 0;
  return s.split(/\r\n|\r|\n/).length;
}

function shouldFoldPaste(s: string): boolean {
  return s.length >= LONG_PASTE_MIN_CHARS || lineCount(s) >= LONG_PASTE_MIN_LINES;
}

export function renderPastedBlock(block: PastedBlock): string {
  return `${block.label}\n\n--- Begin ${block.label} ---\n${block.text}\n--- End ${block.label} ---`;
}

/**
 * usePasteHandler manages large-text paste folding: when the user pastes
 * multi-line / long text, it collapses it into a labelled block that
 * expands inline only when the user views it. The block reference is
 * submitted to the model as a fenced section.
 */
export function usePasteHandler() {
  const t = useT();
  const [pastedBlocks, setPastedBlocks] = useState<PastedBlock[]>([]);
  const [openPastedLabels, setOpenPastedLabels] = useState<string[]>([]);
  const pastedBlocksRef = useRef<PastedBlock[]>([]);
  const nextPasteId = useRef(1);

  const clear = () => {
    pastedBlocksRef.current = [];
    setPastedBlocks([]);
    setOpenPastedLabels([]);
  };

  /** Returns true when the clipboard text should be folded into a labelled block. */
  const tryFoldPaste = (clipboardText: string): boolean => {
    if (!shouldFoldPaste(clipboardText)) return false;
    const id = nextPasteId.current++;
    const lines = lineCount(clipboardText);
    const label = t("composer.pastedLabel", { id, lines });
    const block: PastedBlock = { label, text: clipboardText };
    pastedBlocksRef.current = [...pastedBlocksRef.current, block];
    setPastedBlocks((prev) => [...prev, block]);
    return true;
  };

  const expandBlocks = (displayText: string): string => {
    let expanded = displayText;
    for (const block of pastedBlocksRef.current) {
      if (expanded.includes(block.label)) {
        expanded = expanded.split(block.label).join(renderPastedBlock(block));
      }
    }
    return expanded;
  };

  /** Pasted blocks whose labels still appear in the current text. */
  const activeBlocks = pastedBlocks.filter(() => true); // caller filters by text.includes

  const togglePreview = (label: string) => {
    setOpenPastedLabels((prev) =>
      prev.includes(label) ? prev.filter((x) => x !== label) : [...prev, label],
    );
  };

  const removeBlock = (block: PastedBlock) => {
    pastedBlocksRef.current = pastedBlocksRef.current.filter((x) => x.label !== block.label);
    setPastedBlocks((prev) => prev.filter((x) => x.label !== block.label));
    setOpenPastedLabels((prev) => prev.filter((x) => x !== block.label));
  };

  return {
    pastedBlocks,
    openPastedLabels,
    pendingPaste: 0, // kept for compatibility, handled by caller
    nextPasteId,
    clear,
    tryFoldPaste,
    expandBlocks,
    activeBlocks,
    togglePreview,
    removeBlock,
    setPendingPaste: () => {}, // no-op for now
  };
}
