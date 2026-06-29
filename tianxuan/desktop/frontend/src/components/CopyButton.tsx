import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { useT } from "../lib/i18n";

// CopyButton copies text to the clipboard on click and briefly flips to a check.
// Falls back through: navigator.clipboard → Wails runtime → execCommand.
// (Design adopted from DeepSeek-Reasonix-V1.12)
function fallbackCopyText(value: string): boolean {
  const activeElement = document.activeElement;
  const selection = document.getSelection();
  const ranges: Range[] = [];
  if (selection) {
    for (let index = 0; index < selection.rangeCount; index += 1) {
      ranges.push(selection.getRangeAt(index));
    }
  }
  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.inset = "0 auto auto 0";
  textarea.style.width = "1px";
  textarea.style.height = "1px";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } finally {
    textarea.remove();
    if (selection) {
      selection.removeAllRanges();
      for (const range of ranges) selection.addRange(range);
    }
    if (activeElement instanceof HTMLElement) activeElement.focus();
  }
  return ok;
}

async function writeClipboard(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value);
    return;
  } catch { /* try Wails runtime next */ }
  try {
    if (typeof window !== "undefined" && (window as any).runtime?.ClipboardSetText?.(value)) return;
  } catch { /* runtime unavailable */ }
  if (fallbackCopyText(value)) return;
  throw new Error("clipboard unavailable");
}

export function CopyButton({
  text,
  getText,
  className,
  label,
  showInlineLabel = true,
  ghost,
}: {
  text?: string;
  getText?: () => string | Promise<string>;
  className?: string;
  label?: string;
  showInlineLabel?: boolean;
  ghost?: boolean;
}) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);
  const actionLabel = label ?? t("msg.copy");
  const stateLabel = copied ? t("msg.copied") : actionLabel;

  useEffect(() => {
    return () => {
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
    };
  }, []);

  const copy = async () => {
    try {
      const value = getText ? await getText() : text ?? "";
      await writeClipboard(value);
      setCopied(true);
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => {
        setCopied(false);
        timerRef.current = null;
      }, 1200);
    } catch {
      /* clipboard unavailable — silently ignore */
    }
  };
  return (
    <button
      className={`inline-flex items-center gap-1 rounded-md py-0.5 px-1.5 text-[11px] cursor-pointer transition-[color,border-color] duration-[var(--dur-fast)] ${
        ghost
          ? "bg-transparent border-0 text-fg-faint/50 hover:text-fg-faint"
          : "bg-bg-elev-2 border border-border text-fg-faint hover:text-fg hover:border-fg-faint"
      } ${className ?? ""}`}
      onClick={copy}
      title={actionLabel}
      aria-label={stateLabel}
      type="button"
    >
      {copied ? <Check size={13} /> : <Copy size={13} />}
      {showInlineLabel && (
        <span className="leading-none">{stateLabel}</span>
      )}
    </button>
  );
}
