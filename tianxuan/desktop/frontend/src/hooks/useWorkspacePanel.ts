import { useCallback, useMemo, useState } from "react";
import type { PointerEvent as ReactPointerEvent, KeyboardEvent } from "react";
import {
  WORKSPACE_PANEL_MIN_WIDTH, WORKSPACE_PANEL_MAX_WIDTH, WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH,
  WORKSPACE_FILE_TREE_PANEL_MAX_WIDTH,
  clampWorkspacePanelWidth, clampWorkspaceFileTreePanelWidth,
  loadWorkspacePanelWidth, saveWorkspacePanelWidth,
  loadWorkspaceFileTreePanelWidth, saveWorkspaceFileTreePanelWidth,
} from "./useLayoutSizes";

export function useWorkspacePanel(effectiveSidebarWidth: number, viewportWidth: number) {
  const [workspacePanelOpen, setWorkspacePanelOpen] = useState(true);
  const [workspacePanelWidth, setWorkspacePanelWidth] = useState(loadWorkspacePanelWidth);
  const [workspaceFileTreePanelWidth, setWorkspaceFileTreePanelWidth] = useState(loadWorkspaceFileTreePanelWidth);
  const [workspacePanelResizing, setWorkspacePanelResizing] = useState(false);
  const [workspacePanelMaximized, setWorkspacePanelMaximized] = useState(false);
  const [workspacePreviewModeActive, setWorkspacePreviewModeActive] = useState(false);

  const effectiveWorkspacePanelWidth = useMemo(
    () => workspacePreviewModeActive
      ? clampWorkspacePanelWidth(workspacePanelWidth, effectiveSidebarWidth, viewportWidth)
      : clampWorkspaceFileTreePanelWidth(workspaceFileTreePanelWidth, effectiveSidebarWidth, viewportWidth),
    [effectiveSidebarWidth, viewportWidth, workspaceFileTreePanelWidth, workspacePanelWidth, workspacePreviewModeActive],
  );

  const setSavedWorkspacePanelWidth = useCallback((width: number) => {
    if (workspacePreviewModeActive) {
      const next = clampWorkspacePanelWidth(width, effectiveSidebarWidth, viewportWidth);
      setWorkspacePanelWidth(next);
      saveWorkspacePanelWidth(next);
    } else {
      const next = clampWorkspaceFileTreePanelWidth(width, effectiveSidebarWidth, viewportWidth);
      setWorkspaceFileTreePanelWidth(next);
      saveWorkspaceFileTreePanelWidth(next);
    }
  }, [effectiveSidebarWidth, viewportWidth, workspacePreviewModeActive]);

  const startWorkspacePanelResize = useCallback(
    (e: ReactPointerEvent<HTMLButtonElement>) => {
      if (!workspacePanelOpen || workspacePanelMaximized) return;
      e.preventDefault();
      setWorkspacePanelResizing(true);
      let nextWidth = effectiveWorkspacePanelWidth;
      const clampFn = workspacePreviewModeActive ? clampWorkspacePanelWidth : clampWorkspaceFileTreePanelWidth;
      const onMove = (me: PointerEvent) => {
        nextWidth = clampFn(window.innerWidth - me.clientX, effectiveSidebarWidth, window.innerWidth);
        if (workspacePreviewModeActive) setWorkspacePanelWidth(nextWidth);
        else setWorkspaceFileTreePanelWidth(nextWidth);
      };
      const onDone = () => {
        if (workspacePreviewModeActive) { setWorkspacePanelWidth(nextWidth); saveWorkspacePanelWidth(nextWidth); }
        else { setWorkspaceFileTreePanelWidth(nextWidth); saveWorkspaceFileTreePanelWidth(nextWidth); }
        setWorkspacePanelResizing(false);
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
    [effectiveSidebarWidth, effectiveWorkspacePanelWidth, workspacePanelMaximized, workspacePanelOpen, workspacePreviewModeActive],
  );

  const resizeWorkspacePanelWithKeyboard = useCallback(
    (e: KeyboardEvent<HTMLButtonElement>) => {
      if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
        e.preventDefault();
        setSavedWorkspacePanelWidth(effectiveWorkspacePanelWidth + (e.key === "ArrowLeft" ? 16 : -16));
      } else if (e.key === "Home") {
        e.preventDefault();
        setSavedWorkspacePanelWidth(workspacePreviewModeActive ? WORKSPACE_PANEL_MIN_WIDTH : WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH);
      } else if (e.key === "End") {
        e.preventDefault();
        setSavedWorkspacePanelWidth(workspacePreviewModeActive ? WORKSPACE_PANEL_MAX_WIDTH : WORKSPACE_FILE_TREE_PANEL_MAX_WIDTH);
      }
    },
    [effectiveWorkspacePanelWidth, setSavedWorkspacePanelWidth, workspacePreviewModeActive],
  );

  const setWorkspacePanel = useCallback((open: boolean) => {
    setWorkspacePanelOpen(open);
    if (!open) { setWorkspacePanelMaximized(false); setWorkspacePreviewModeActive(false); }
  }, []);

  const toggleWorkspacePanel = useCallback(() => setWorkspacePanelOpen((o) => !o), []);

  return {
    workspacePanelOpen, workspacePanelWidth, workspaceFileTreePanelWidth,
    workspacePanelResizing, workspacePanelMaximized, workspacePreviewModeActive,
    effectiveWorkspacePanelWidth,
    setWorkspacePanelOpen, setWorkspacePanelWidth, setWorkspaceFileTreePanelWidth,
    setWorkspacePanelResizing, setWorkspacePanelMaximized, setWorkspacePreviewModeActive,
    setSavedWorkspacePanelWidth, startWorkspacePanelResize, resizeWorkspacePanelWithKeyboard,
    setWorkspacePanel, toggleWorkspacePanel,
  };
}
