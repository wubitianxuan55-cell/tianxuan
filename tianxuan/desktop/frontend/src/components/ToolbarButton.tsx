import { memo, type ReactNode } from "react";

/** 统一的工具栏按钮 — 5 处重复 className 合并为单个组件 */
export const ToolbarButton = memo(function ToolbarButton({
  onClick,
  disabled,
  title,
  children,
}: {
  onClick: () => void;
  disabled?: boolean;
  title?: string;
  children: ReactNode;
}) {
  return (
    <button
      className="toolbar-btn no-drag"
      onClick={onClick}
      disabled={disabled}
      title={title}
    >
      {children}
    </button>
  );
});
