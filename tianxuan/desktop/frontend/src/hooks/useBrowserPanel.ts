import { useCallback, useMemo, useState } from "react";
import type { KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import {
  BROWSER_PANEL_DEFAULT_WIDTH,
  BROWSER_PANEL_MAX_WIDTH,
  BROWSER_PANEL_MIN_WIDTH,
  clampBrowserPanelWidth,
  loadBrowserPanelWidth,
  saveBrowserPanelWidth,
} from "./useLayoutSizes";

/**
 * 浏览器右侧分栏的宽度状态与拖拽调整。与 useWorkspacePanel 同模式：
 * pointer 拖拽 + 键盘（方向键/Home/End）+ 双击恢复默认，宽度持久化到
 * localStorage。修复"浏览器分栏宽度写死 CSS、无法拖动变宽"。
 */
export function useBrowserPanel(effectiveSidebarWidth: number, viewportWidth: number) {
  const [browserPanelWidth, setBrowserPanelWidth] = useState(loadBrowserPanelWidth);
  const [browserPanelResizing, setBrowserPanelResizing] = useState(false);

  const effectiveBrowserPanelWidth = useMemo(
    () => clampBrowserPanelWidth(browserPanelWidth, effectiveSidebarWidth, viewportWidth),
    [browserPanelWidth, effectiveSidebarWidth, viewportWidth],
  );

  const startBrowserPanelResize = useCallback(
    (e: ReactPointerEvent<HTMLButtonElement>) => {
      e.preventDefault();
      setBrowserPanelResizing(true);
      let nextWidth = effectiveBrowserPanelWidth;
      const onMove = (me: PointerEvent) => {
        nextWidth = clampBrowserPanelWidth(
          window.innerWidth - me.clientX,
          effectiveSidebarWidth,
          window.innerWidth,
        );
        setBrowserPanelWidth(nextWidth);
      };
      const onDone = () => {
        setBrowserPanelWidth(nextWidth);
        saveBrowserPanelWidth(nextWidth);
        setBrowserPanelResizing(false);
        window.removeEventListener("pointermove", onMove);
        window.removeEventListener("pointerup", onDone);
        window.removeEventListener("pointercancel", onDone);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      };
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
      window.addEventListener("pointermove", onMove);
      window.addEventListener("pointerup", onDone);
      window.addEventListener("pointercancel", onDone);
    },
    [effectiveBrowserPanelWidth, effectiveSidebarWidth],
  );

  const resizeBrowserPanelWithKeyboard = useCallback(
    (e: KeyboardEvent<HTMLButtonElement>) => {
      if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
        e.preventDefault();
        const next = effectiveBrowserPanelWidth + (e.key === "ArrowLeft" ? 16 : -16);
        setBrowserPanelWidth(clampBrowserPanelWidth(next, effectiveSidebarWidth, viewportWidth));
      } else if (e.key === "Home") {
        e.preventDefault();
        setBrowserPanelWidth(BROWSER_PANEL_MIN_WIDTH);
      } else if (e.key === "End") {
        e.preventDefault();
        setBrowserPanelWidth(clampBrowserPanelWidth(BROWSER_PANEL_MAX_WIDTH, effectiveSidebarWidth, viewportWidth));
      }
    },
    [effectiveBrowserPanelWidth, effectiveSidebarWidth, viewportWidth],
  );

  const resetBrowserPanelWidth = useCallback(() => {
    setBrowserPanelWidth(BROWSER_PANEL_DEFAULT_WIDTH);
    saveBrowserPanelWidth(BROWSER_PANEL_DEFAULT_WIDTH);
  }, []);

  return {
    browserPanelWidth,
    browserPanelResizing,
    effectiveBrowserPanelWidth,
    setBrowserPanelWidth,
    startBrowserPanelResize,
    resizeBrowserPanelWithKeyboard,
    resetBrowserPanelWidth,
  };
}
