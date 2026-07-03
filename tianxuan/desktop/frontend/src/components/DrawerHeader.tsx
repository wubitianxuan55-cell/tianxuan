import { memo, type ReactNode } from "react";
import { CloseButton } from "./CloseButton";

/** DrawerHeader — standardized header for all slide-out drawer panels.
 *  Provides uniform layout, spacing, and close-button position. */
export const DrawerHeader = memo(function DrawerHeader({
  onClose,
  children,
}: {
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <header className="flex items-center justify-between shrink-0 px-4 py-3.5 bg-bg-elev border-b border-border">
      <div className="flex items-center gap-2.5 min-w-0">{children}</div>
      <CloseButton onClick={onClose} />
    </header>
  );
});

/** DrawerTitle — semantic title text inside a DrawerHeader. */
export const DrawerTitle = memo(function DrawerTitle({ text }: { text: string }) {
  return <span className="text-[15px] font-semibold text-fg">{text}</span>;
});

/** DrawerSubtitle — supplementary metadata below or beside the title. */
export const DrawerSubtitle = memo(function DrawerSubtitle({ text }: { text: string }) {
  return <div className="mt-[3px] text-fg-faint text-[11px]">{text}</div>;
});
