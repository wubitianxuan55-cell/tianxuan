import { useEffect } from "react";
import type { MemoryView, SessionMeta } from "../lib/types";

interface ShortcutDeps {
  running: boolean;
  capsOpen: boolean;
  settingsOpen: boolean;
  memView: MemoryView | null;
  histView: SessionMeta[] | null;
  showPlan: boolean;
  workspacePanelOpen: boolean;
  setCapsOpen: (v: boolean) => void;
  setSettingsOpen: (v: boolean) => void;
  setMemView: (v: MemoryView | null) => void;
  setHistView: (v: SessionMeta[] | null) => void;
  setShowPlan: (v: boolean) => void;
  setWorkspacePanel: (v: boolean) => void;
  startNewSession: () => Promise<void>;
  openMemory: () => Promise<void>;
  openHistory: () => Promise<void>;
  toggleSidebar: () => void;
  toggleWorkspacePanel: () => void;
  setPaletteOpen: (v: boolean) => void;
}

export function useGlobalShortcuts(deps: ShortcutDeps) {
  useEffect(() => {
    const onKey = (e: Event) => {
      const ke = e as globalThis.KeyboardEvent;
      const mod = ke.ctrlKey || ke.metaKey,
        t = ke.target as HTMLElement;
      const inInput =
        t.tagName === "INPUT" ||
        t.tagName === "TEXTAREA" ||
        t.isContentEditable;
      if (ke.key === "Escape" && !inInput && !deps.running) {
        if (deps.capsOpen) {
          ke.preventDefault();
          deps.setCapsOpen(false);
          return;
        }
        if (deps.settingsOpen) {
          ke.preventDefault();
          deps.setSettingsOpen(false);
          return;
        }
        if (deps.memView !== null) {
          ke.preventDefault();
          deps.setMemView(null);
          return;
        }
        if (deps.histView !== null) {
          ke.preventDefault();
          deps.setHistView(null);
          return;
        }
        if (deps.showPlan) {
          ke.preventDefault();
          deps.setShowPlan(false);
          return;
        }
        if (deps.workspacePanelOpen) {
          ke.preventDefault();
          deps.setWorkspacePanel(false);
          return;
        }
        return;
      }
      if (!mod) return;
      if (ke.key === "n" && !deps.running) {
        ke.preventDefault();
        void deps.startNewSession();
        return;
      }
      if (ke.key === "k") {
        ke.preventDefault();
        deps.setPaletteOpen(true);
        return;
      }
      if (ke.key === ",") {
        ke.preventDefault();
        deps.setSettingsOpen(true);
        return;
      }
      if (ke.key === "M" && ke.shiftKey) {
        ke.preventDefault();
        void deps.openMemory();
        return;
      }
      if (ke.key === "H" && ke.shiftKey) {
        ke.preventDefault();
        void deps.openHistory();
        return;
      }
      if (ke.key === "b") {
        ke.preventDefault();
        deps.toggleSidebar();
        return;
      }
      if (ke.key === "j") {
        ke.preventDefault();
        deps.toggleWorkspacePanel();
        return;
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [
    deps.running,
    deps.capsOpen,
    deps.settingsOpen,
    deps.memView,
    deps.histView,
    deps.showPlan,
    deps.workspacePanelOpen,
  ]);
}
