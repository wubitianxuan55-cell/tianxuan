import { memo } from "react";

export const EmptyState = memo(function EmptyState(p: { message: string }) {
  return (
    <div className="py-14 text-center">
      <div className="text-fg-faint/40 text-[13px]">{p.message}</div>
    </div>
  );
});
