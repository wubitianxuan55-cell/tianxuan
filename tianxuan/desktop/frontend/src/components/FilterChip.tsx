import { memo } from "react";

export const FilterChip = memo(function FilterChip(p: {
  active: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      className={`ds-chip ${p.active ? "ds-chip--accent" : "ds-chip--muted"}`}
      onClick={p.onClick}
      type="button"
    >
      {p.label}
    </button>
  );
});
