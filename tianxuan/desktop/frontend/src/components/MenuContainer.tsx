import { useEffect, useRef, type ReactNode } from "react";

/** 三个菜单(SlashMenu/FileMenu/ArgMenu)共享的容器：定位、外观、动画、键盘提示栏 */
export function MenuContainer({
  children,
  count,
  hint,
}: {
  children: ReactNode;
  count: number;
  hint?: string;
}) {
  return (
    <div
      className="absolute bottom-[calc(100%+6px)] left-0 right-0 max-h-[280px] overflow-y-auto bg-bg-elev border border-border rounded-[10px] p-[5px] shadow-[0_12px_32px_rgba(0,0,0,0.4)] z-20 animate-[menu-in_0.12s_ease]"
      role="listbox"
    >
      {children}
      {count > 0 && (
        <div className="flex items-center gap-2 px-2 py-1.5 text-[10px] text-fg-faint/50 border-t border-border-soft mt-0.5">
          <span>↑↓ 导航</span>
          <span>↵ 选择</span>
          <span>Esc 关闭</span>
          {hint && (
            <>
              <span className="text-border mx-0.5">·</span>
              <span>{hint}</span>
            </>
          )}
        </div>
      )}
    </div>
  );
}

/** 用 ref 驱动选中项滚动到可见区域 */
export function useMenuScroll(activeIndex: number) {
  const activeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);
  return activeRef;
}
