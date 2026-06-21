import type { ReactNode } from "react";

// Simple CSS-only tooltip. Renders children with a data-tooltip attribute
// that CSS turns into a centered tooltip on hover. For more complex positioning
// (side/fill/portal), see Reasonix's full Tooltip component.
export function Tooltip({
  label,
  children,
  className,
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span className={`tooltip ${className ?? ""}`} data-tooltip={label}>
      {children}
    </span>
  );
}
