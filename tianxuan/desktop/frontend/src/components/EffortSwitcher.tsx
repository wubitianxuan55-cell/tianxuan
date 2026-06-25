import { useRef, useState } from "react";
import { AnchoredPopover } from "./AnchoredPopover";

const EFFORT_LEVELS = [
  { id: "fast" as const, label: "快速", temp: 0.1, desc: "低延迟，适合简单任务" },
  { id: "normal" as const, label: "标准", temp: 0.3, desc: "平衡速度与质量" },
  { id: "deep" as const, label: "深度", temp: 0.7, desc: "更长的思考链，适合复杂问题" },
];

interface EffortSwitcherProps {
  value: "fast" | "normal" | "deep";
  onChange: (level: "fast" | "normal" | "deep") => void;
  disabled?: boolean;
}

export function EffortSwitcher({ value, onChange, disabled }: EffortSwitcherProps) {
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);
  const current = EFFORT_LEVELS.find((l) => l.id === value) ?? EFFORT_LEVELS[1];

  return (
    <>
      <button
        ref={anchorRef}
        className="effort-switcher-btn"
        onClick={() => !disabled && setOpen((v) => !v)}
        disabled={disabled}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 4,
          padding: "2px 8px",
          border: "1px solid var(--border-soft)",
          borderRadius: 5,
          background: "transparent",
          color: "var(--fg-faint)",
          fontSize: 11,
          cursor: disabled ? "default" : "pointer",
          opacity: disabled ? 0.4 : 1,
          transition: "color var(--dur-fast), background var(--dur-fast)",
        }}
        onMouseEnter={(e) => { if (!disabled) e.currentTarget.style.color = "var(--fg-dim)"; }}
        onMouseLeave={(e) => { e.currentTarget.style.color = "var(--fg-faint)"; }}
      >
        <span>{current.label}</span>
        <span style={{ fontSize: 9 }}>▼</span>
      </button>
      <AnchoredPopover
        open={open}
        anchorRef={anchorRef}
        onClose={() => setOpen(false)}
        className="effort-switcher-popover"
        placement="bottom"
      >
        <div
          style={{
            width: 200,
            padding: 4,
            background: "var(--bg-elev-2)",
            border: "1px solid var(--border)",
            borderRadius: "var(--radius)",
            boxShadow: "var(--ds-shadow-dropdown)",
          }}
        >
          {EFFORT_LEVELS.map((level) => (
            <button
              key={level.id}
              className="effort-switcher-option"
              onClick={() => {
                onChange(level.id);
                setOpen(false);
              }}
              style={{
                display: "flex",
                flexDirection: "column",
                gap: 2,
                width: "100%",
                padding: "6px 8px",
                border: "none",
                borderRadius: 5,
                background: value === level.id ? "var(--accent-soft)" : "transparent",
                color: value === level.id ? "var(--accent)" : "var(--fg)",
                fontSize: 12,
                cursor: "pointer",
                textAlign: "left",
                transition: "background var(--dur-fast)",
              }}
              onMouseEnter={(e) => {
                if (value !== level.id) e.currentTarget.style.background = "var(--sidebar-hover)";
              }}
              onMouseLeave={(e) => {
                if (value !== level.id) e.currentTarget.style.background = "transparent";
              }}
            >
              <span style={{ fontWeight: value === level.id ? 600 : 400 }}>{level.label}</span>
              <span style={{ fontSize: 11, color: "var(--fg-faint)" }}>{level.desc}</span>
            </button>
          ))}
        </div>
      </AnchoredPopover>
    </>
  );
}
