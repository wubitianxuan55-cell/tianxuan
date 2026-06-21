import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { useT } from "../lib/i18n";

// CopyButton copies `text` to the clipboard on click and briefly flips to a check.
// Falls back to document.execCommand("copy") when navigator.clipboard is unavailable
// (e.g. in some Wails webview configurations).
function copyFallback(text: string): boolean {
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    ta.style.top = "-9999px";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

export function CopyButton({
  text,
  className,
  label,
}: {
  text: string;
  className?: string;
  label?: string;
}) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    let ok = false;
    try {
      await navigator.clipboard.writeText(text);
      ok = true;
    } catch {
      ok = copyFallback(text);
    }
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    }
  };
  return (
    <button
      className={`inline-flex items-center gap-1 bg-bg-elev-2 border border-border text-fg-faint rounded-md py-0.5 px-1.5 text-[11px] cursor-pointer transition-[color,border-color] duration-[var(--dur-fast)] hover:text-fg hover:border-fg-faint ${className ?? ""}`}
      onClick={copy}
      title={t("msg.copy")}
      aria-label={t("msg.copy")}
      type="button"
    >
      {copied ? <Check size={13} /> : <Copy size={13} />}
      {label && <span className="leading-none">{copied ? t("msg.copied") : label}</span>}
    </button>
  );
}
