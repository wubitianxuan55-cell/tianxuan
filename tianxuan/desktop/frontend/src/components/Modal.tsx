import { useEffect, useState, type ReactNode } from "react";

/**
 * Modal — 居中弹窗容器
 *
 * 特性：
 * - scale+fade 入场/出场动画（入场 200ms, 出场 120ms）
 * - 半透明遮罩，点击关闭
 * - ESC 键关闭
 * - aria-modal / role="dialog" 可访问性
 * - wide 模式控制宽度（默认 max-w-lg, wide 模式 max-w-5xl）
 */
export function Modal({
  children,
  onClose,
  wide = false,
}: {
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
}) {
  const [exiting, setExiting] = useState(false);

  const handleClose = () => {
    if (exiting) return;
    setExiting(true);
    setTimeout(() => onClose(), 120);
  };

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") handleClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div
      className="fixed inset-0 z-90 flex items-center justify-center"
      role="dialog"
      aria-modal="true"
    >
      {/* 半透明遮罩 */}
      <div className="absolute inset-0 bg-black/50" onClick={handleClose} />

      {/* 弹窗面板 */}
      <div
        className={
          "relative flex flex-col bg-bg-elev border border-border rounded-xl shadow-[var(--ds-shadow-panel)] " +
          "max-h-[88vh] w-[88vw] " +
          (wide ? "max-w-5xl" : "max-w-lg") +
          " " +
          (exiting ? "anim-modal-out" : "anim-modal-in")
        }
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}
