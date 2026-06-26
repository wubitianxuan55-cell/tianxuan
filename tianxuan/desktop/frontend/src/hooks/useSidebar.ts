import { useCallback, useRef, useState } from "react";
import type { KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import {
  SIDEBAR_COLLAPSED_WIDTH, SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH,
  clampSidebarWidth, loadSidebarCollapsed, saveSidebarCollapsed,
  loadSidebarWidth, saveSidebarWidth,
} from "./useLayoutSizes";

export function useSidebar() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(loadSidebarCollapsed);
  const [sidebarWidth, setSidebarWidth] = useState(loadSidebarWidth);
  const [sidebarResizing, setSidebarResizing] = useState(false);
  const sidebarBeforeRef = useRef<boolean | null>(null);
  const effectiveSidebarWidth = sidebarCollapsed ? SIDEBAR_COLLAPSED_WIDTH : sidebarWidth;
  const sidebarWidthRef = useRef(effectiveSidebarWidth);
  sidebarWidthRef.current = effectiveSidebarWidth;

  const toggleSidebar = useCallback(() => {
    sidebarBeforeRef.current = null;
    setSidebarCollapsed((c) => {
      saveSidebarCollapsed(!c);
      return !c;
    });
  }, []);

  const setExpandedSidebarWidth = useCallback((w: number) => {
    const next = clampSidebarWidth(w);
    setSidebarWidth(next);
    saveSidebarWidth(next);
  }, []);

  const startSidebarResize = useCallback(
    (e: ReactPointerEvent<HTMLButtonElement>) => {
      if (sidebarCollapsed) return;
      e.preventDefault();
      setSidebarResizing(true);
      let nextWidth = sidebarWidth;
      const onMove = (me: PointerEvent) => {
        nextWidth = clampSidebarWidth(me.clientX);
        setSidebarWidth(nextWidth);
      };
      const onDone = () => {
        setSidebarWidth(nextWidth);
        saveSidebarWidth(nextWidth);
        setSidebarResizing(false);
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
    [sidebarCollapsed, sidebarWidth],
  );

  const resizeSidebarWithKeyboard = useCallback(
    (e: KeyboardEvent<HTMLButtonElement>) => {
      if (sidebarCollapsed) return;
      if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
        e.preventDefault();
        setExpandedSidebarWidth(sidebarWidth + (e.key === "ArrowRight" ? 16 : -16));
      } else if (e.key === "Home") {
        e.preventDefault();
        setExpandedSidebarWidth(SIDEBAR_MIN_WIDTH);
      } else if (e.key === "End") {
        e.preventDefault();
        setExpandedSidebarWidth(SIDEBAR_MAX_WIDTH);
      }
    },
    [setExpandedSidebarWidth, sidebarCollapsed, sidebarWidth],
  );

  // Preview mode: auto-collapse sidebar to make room, but allow manual re-expand.
  // On preview exit, restore to the remembered collapse state.
  const handleWorkspacePreviewModeChange = useCallback((active: boolean) => {
    if (active) {
      if (sidebarBeforeRef.current === null) sidebarBeforeRef.current = sidebarCollapsed;
      if (!sidebarCollapsed) {
        setSidebarCollapsed(true);
        saveSidebarCollapsed(true);
      }
    } else {
      const restore = sidebarBeforeRef.current;
      sidebarBeforeRef.current = null;
      if (restore !== null && restore !== sidebarCollapsed) {
        setSidebarCollapsed(restore);
        saveSidebarCollapsed(restore);
      }
    }
  }, [sidebarCollapsed]);

  return {
    sidebarCollapsed, sidebarWidth, sidebarResizing, effectiveSidebarWidth,
    sidebarWidthRef,
    toggleSidebar, setExpandedSidebarWidth, startSidebarResize,
    resizeSidebarWithKeyboard, handleWorkspacePreviewModeChange,
    setSidebarWidth, setSidebarCollapsed,
  };
}
