import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import {
  ChevronDown,
  ChevronRight,
  Columns2,
  FileText,
  Folder,
  Maximize2,
  Minimize2,
  PanelRightClose,
  Plus,
  Search,
  X,
} from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { loadLayoutSize, saveLayoutSize } from "../lib/layoutPreferences";
import type { DirEntry, FilePreview } from "../lib/types";
import { CodeViewer } from "./CodeViewer";
import { Markdown } from "./Markdown";

const WORKSPACE_TREE_MIN_WIDTH = 220;
const WORKSPACE_TREE_DEFAULT_WIDTH = WORKSPACE_TREE_MIN_WIDTH;
const WORKSPACE_TREE_MAX_WIDTH = 420;
const WORKSPACE_PREVIEW_MIN_WIDTH = 420;

function clampWorkspaceTreeWidth(width: number, panelWidth?: number): number {
  const maxForPanel =
    typeof panelWidth === "number" && Number.isFinite(panelWidth)
      ? Math.max(WORKSPACE_TREE_MIN_WIDTH, panelWidth - WORKSPACE_PREVIEW_MIN_WIDTH)
      : WORKSPACE_TREE_MAX_WIDTH;
  const max = Math.min(WORKSPACE_TREE_MAX_WIDTH, maxForPanel);
  return Math.min(max, Math.max(WORKSPACE_TREE_MIN_WIDTH, Math.round(width)));
}

function loadWorkspaceTreeWidth(): number {
  return loadLayoutSize("workspaceTreeWidth", WORKSPACE_TREE_DEFAULT_WIDTH, clampWorkspaceTreeWidth);
}

function saveWorkspaceTreeWidth(width: number): void {
  saveLayoutSize("workspaceTreeWidth", width);
}

function entryPath(dir: string, entry: DirEntry): string {
  const prefix = dir === "" || dir.endsWith("/") ? dir : dir + "/";
  return prefix + entry.name + (entry.isDir ? "/" : "");
}

function basename(path: string): string {
  const parts = path.split("/").filter(Boolean);
  return parts[parts.length - 1] ?? "";
}

function parentPath(path: string): string {
  const clean = path.replace(/\/$/, "");
  const parts = clean.split("/").filter(Boolean);
  return parts.slice(0, -1).join("/");
}

function parentDirs(path: string): string[] {
  const parts = path.split("/").filter(Boolean);
  const dirs: string[] = [""];
  let acc = "";
  for (let i = 0; i < parts.length - 1; i++) {
    acc += parts[i] + "/";
    dirs.push(acc);
  }
  return dirs;
}

function languageFor(path: string): string | undefined {
  const name = basename(path).toLowerCase();
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : name;
  const byExt: Record<string, string> = {
    css: "css",
    go: "go",
    html: "html",
    js: "javascript",
    json: "json",
    jsx: "jsx",
    md: "markdown",
    mjs: "javascript",
    php: "php",
    py: "python",
    rb: "ruby",
    rs: "rust",
    sh: "bash",
    sql: "sql",
    svg: "xml",
    toml: "toml",
    ts: "typescript",
    tsx: "tsx",
    yaml: "yaml",
    yml: "yaml",
  };
  return byExt[ext];
}

function shortCwd(cwd?: string): string {
  if (!cwd) return "";
  const parts = cwd.split("/").filter(Boolean);
  if (parts.length <= 2) return cwd;
  return "…/" + parts.slice(-2).join("/");
}

function formatBytes(n: number): string {
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${Math.ceil(n / 1024)} KB`;
  return `${n} B`;
}

export function WorkspacePanel({
  open,
  cwd,
  maximized,
  panelWidth,
  onClose,
  onToggleMaximized,
  onPreviewModeChange,
}: {
  open: boolean;
  cwd?: string;
  maximized: boolean;
  panelWidth?: number;
  onClose: () => void;
  onToggleMaximized: () => void;
  onPreviewModeChange?: (active: boolean) => void;
}) {
  const t = useT();
  const panelRef = useRef<HTMLElement>(null);
  const filterRef = useRef<HTMLInputElement>(null);
  const [entriesByDir, setEntriesByDir] = useState<Record<string, DirEntry[]>>({});
  const [openDirs, setOpenDirs] = useState<Set<string>>(() => new Set([""]));
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [openTabs, setOpenTabs] = useState<string[]>([]);
  const [preview, setPreview] = useState<FilePreview | null>(null);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [filter, setFilter] = useState("");
  const [treeVisible, setTreeVisible] = useState(true);
  const [treeWidth, setTreeWidth] = useState(loadWorkspaceTreeWidth);
  const [treeResizing, setTreeResizing] = useState(false);

  const loadDir = useCallback(async (dir: string) => {
    const entries = await app.ListDir(dir).catch(() => []);
    setEntriesByDir((prev) => ({ ...prev, [dir]: entries ?? [] }));
  }, []);

  const selectFile = useCallback(
    (path: string) => {
      setSelectedPath(path);
      setFilter("");
      setOpenTabs((tabs) => (tabs.includes(path) ? tabs : [...tabs, path]));
      const dirs = parentDirs(path);
      setOpenDirs((prev) => new Set([...Array.from(prev), ...dirs]));
      dirs.forEach((dir) => {
        if (!entriesByDir[dir]) void loadDir(dir);
      });
    },
    [entriesByDir, loadDir],
  );

  useEffect(() => {
    if (!open) return;
    setEntriesByDir({});
    setOpenDirs(new Set([""]));
    setSelectedPath(null);
    setOpenTabs([]);
    setPreview(null);
    setFilter("");
    setTreeVisible(true);
    void loadDir("");
  }, [cwd, loadDir, open]);

  const refreshSelected = useCallback(() => {
    if (!selectedPath) return;
    let live = true;
    setLoadingPreview(true);
    app
      .ReadFile(selectedPath)
      .then((next) => {
        if (live) setPreview(next);
      })
      .catch((err) => {
        if (live) {
          setPreview({
            path: selectedPath,
            body: "",
            size: 0,
            truncated: false,
            binary: false,
            err: String(err?.message ?? err),
          });
        }
      })
      .finally(() => {
        if (live) setLoadingPreview(false);
      });
    return () => {
      live = false;
    };
  }, [selectedPath]);

  useEffect(() => {
    if (!open || !selectedPath) return;
    return refreshSelected();
  }, [open, refreshSelected, selectedPath]);

  const toggleDir = useCallback(
    (dir: string) => {
      setOpenDirs((prev) => {
        const next = new Set(prev);
        if (next.has(dir)) {
          next.delete(dir);
        } else {
          next.add(dir);
          if (!entriesByDir[dir]) void loadDir(dir);
        }
        return next;
      });
    },
    [entriesByDir, loadDir],
  );

  const openPickerTab = () => {
    setSelectedPath(null);
    setPreview(null);
    setFilter("");
    setTreeVisible(true);
    requestAnimationFrame(() => filterRef.current?.focus());
  };

  const closeTab = (path: string) => {
    setOpenTabs((tabs) => {
      const next = tabs.filter((tab) => tab !== path);
      if (selectedPath === path) {
        const replacement = next[next.length - 1] ?? null;
        setSelectedPath(replacement);
        if (!replacement) {
          setPreview(null);
          setTreeVisible(true);
        }
      }
      return next;
    });
  };

  const breadcrumbDirs = selectedPath ? parentDirs(selectedPath) : [""];
  const pathParts = selectedPath?.split("/").filter(Boolean) ?? [];
  const flattened = useMemo(() => {
    const rows: { path: string; entry: DirEntry }[] = [];
    for (const [dir, entries] of Object.entries(entriesByDir)) {
      for (const entry of entries) {
        rows.push({ path: entryPath(dir, entry), entry });
      }
    }
    const q = filter.trim().toLowerCase();
    if (!q) return null;
    return rows
      .filter((row) => row.path.toLowerCase().includes(q))
      .sort((a, b) => a.path.localeCompare(b.path));
  }, [entriesByDir, filter]);

  const effectiveTreeWidth = useMemo(() => clampWorkspaceTreeWidth(treeWidth, panelWidth), [panelWidth, treeWidth]);
  const previewVisible = openTabs.length > 0 || selectedPath !== null;
  const previewModeActive = open && previewVisible;

  const panelStyle = useMemo(
    () => ({ "--workspace-tree-width": `${effectiveTreeWidth}px` }) as CSSProperties,
    [effectiveTreeWidth],
  );

  useEffect(() => {
    onPreviewModeChange?.(previewModeActive);
  }, [onPreviewModeChange, previewModeActive]);

  useEffect(() => {
    if (open && !treeVisible && !previewVisible) onClose();
  }, [onClose, open, previewVisible, treeVisible]);

  const hideTreeOrClosePanel = useCallback(() => {
    if (previewVisible) {
      setTreeVisible(false);
    } else {
      onClose();
    }
  }, [onClose, previewVisible]);

  const setSavedTreeWidth = useCallback(
    (width: number) => {
      const next = clampWorkspaceTreeWidth(width, panelWidth);
      setTreeWidth(next);
      saveWorkspaceTreeWidth(next);
    },
    [panelWidth],
  );

  const startTreeResize = useCallback(
    (event: ReactPointerEvent<HTMLButtonElement>) => {
      if (!treeVisible) return;
      const rect = panelRef.current?.getBoundingClientRect();
      if (!rect) return;
      event.preventDefault();
      setTreeResizing(true);
      let nextWidth = effectiveTreeWidth;
      const onMove = (moveEvent: PointerEvent) => {
        nextWidth = clampWorkspaceTreeWidth(rect.right - moveEvent.clientX, rect.width);
        setTreeWidth(nextWidth);
      };
      const onDone = () => {
        setTreeWidth(nextWidth);
        saveWorkspaceTreeWidth(nextWidth);
        setTreeResizing(false);
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
    [effectiveTreeWidth, treeVisible],
  );

  const resizeTreeWithKeyboard = useCallback(
    (event: KeyboardEvent<HTMLButtonElement>) => {
      if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
        event.preventDefault();
        setSavedTreeWidth(effectiveTreeWidth + (event.key === "ArrowLeft" ? 16 : -16));
      } else if (event.key === "Home") {
        event.preventDefault();
        setSavedTreeWidth(WORKSPACE_TREE_MIN_WIDTH);
      } else if (event.key === "End") {
        event.preventDefault();
        setSavedTreeWidth(WORKSPACE_TREE_MAX_WIDTH);
      }
    },
    [effectiveTreeWidth, setSavedTreeWidth],
  );

  if (!open) return null;

  const renderRows = (dir: string, depth: number): JSX.Element[] => {
    const entries = entriesByDir[dir] ?? [];
    return entries.flatMap((entry) => {
      const path = entryPath(dir, entry);
      const isOpen = openDirs.has(path);
      const active = selectedPath === path;
      const row = (
        <button
          className={`w-full min-w-0 h-[30px] flex items-center gap-1.5 border-0 rounded-md bg-transparent text-fg-dim text-[12.5px] text-left cursor-pointer no-drag hover:bg-sidebar-hover hover:text-fg border-l-[3px] ${
            active ? "!border-l-accent bg-sidebar-active !text-fg" : "border-l-transparent"
          }`}
          key={path}
          onClick={() => (entry.isDir ? toggleDir(path) : selectFile(path))}
          title={path}
          style={{ paddingLeft: 8 + depth * 14 }}>
          {entry.isDir ? (
            isOpen ? (
              <ChevronDown size={13} className="w-[13px] h-[13px] shrink-0 text-fg-faint" />
            ) : (
              <ChevronRight size={13} className="w-[13px] h-[13px] shrink-0 text-fg-faint" />
            )
          ) : (
            <span className="w-[13px] h-[13px] shrink-0" />
          )}
          {entry.isDir ? (
            <Folder size={14} className={`shrink-0 ${active ? "text-accent" : "text-fg-dim"}`} />
          ) : (
            <FileText size={14} className={`shrink-0 ${active ? "text-accent" : "text-fg-faint"}`} />
          )}
          <span className={`min-w-0 truncate ${active ? "text-accent font-medium" : ""}`}>{entry.name}</span>
        </button>
      );
      if (!entry.isDir || !isOpen) return [row];
      return [row, ...renderRows(path, depth + 1)];
    });
  };

  const isMarkdown = selectedPath?.toLowerCase().endsWith(".md") ?? false;

  return (
    <aside
      ref={panelRef}
      className={`workspace-panel${treeVisible ? "" : " workspace-panel--tree-hidden"}${previewVisible ? "" : " workspace-panel--preview-hidden"}${treeResizing ? " workspace-panel--tree-resizing" : ""}`}
      aria-label={t("workspace.title")}
      style={panelStyle}
    >
      {previewVisible && <section className="flex flex-col min-w-0 min-h-0 border-r border-border-soft">
        <header className="flex items-center gap-2 h-[50px] px-3 border-b border-border-soft shrink-0">
          <div className="flex items-center gap-1.5 min-w-0 overflow-x-auto overflow-y-hidden flex-1 scrollbar-none">
            {openTabs.map((tab) => (
              <button
                className={`inline-flex items-center gap-1.5 min-w-[112px] max-w-[180px] h-[30px] px-2.5 border rounded-lg bg-transparent text-fg-dim text-[12.5px] font-medium cursor-pointer shrink-0 no-drag transition-[color,background,border] duration-[0.12s] ${
                  selectedPath === tab
                    ? "bg-bg-elev text-fg border-fg-faint/30"
                    : "border-border-soft hover:bg-bg-soft hover:text-fg hover:border-fg-faint"
                }`}
                key={tab}
                onClick={() => setSelectedPath(tab)}
                title={tab}
              >
                <FileText size={14} className="shrink-0" />
                <span className="flex-1 min-w-0 truncate">{basename(tab)}</span>
                <span
                  className="inline-flex items-center justify-center w-4 h-4 rounded text-fg-faint hover:text-err hover:bg-bg-soft shrink-0"
                  role="button"
                  tabIndex={0}
                  title={t("workspace.closeTab")}
                  onClick={(e) => { e.stopPropagation(); closeTab(tab); }}
                  onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); e.stopPropagation(); closeTab(tab); } }}
                >
                  <X size={12} />
                </span>
              </button>
            ))}
            <button className="inline-flex items-center justify-center shrink-0 w-[30px] h-[30px] p-0 border border-border-soft rounded-lg bg-transparent text-fg-faint cursor-pointer no-drag hover:bg-bg-soft hover:text-fg" onClick={openPickerTab} title={t("workspace.newTab")}>
              <Plus size={14} />
            </button>
          </div>

          <div className="flex items-center gap-1.5 shrink-0">
            <button className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-faint cursor-pointer transition-[color,background] duration-[0.12s] hover:text-fg hover:bg-bg-soft disabled:opacity-40 disabled:cursor-default no-drag" onClick={onToggleMaximized} title={maximized ? t("workspace.restore") : t("workspace.maximize")}>
              {maximized ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
            </button>
            <button
              className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-[color,background] duration-[0.12s] hover:text-fg hover:bg-bg-soft no-drag"
              onClick={() => setTreeVisible((value) => !value)}
              title={treeVisible ? t("workspace.hideTree") : t("workspace.showTree")}
            >
              {treeVisible ? <PanelRightClose size={15} /> : <Columns2 size={15} />}
            </button>
          </div>
        </header>

        <div className="flex items-center gap-1.5 min-w-0 px-[18px] py-2 border-b border-border-soft text-[11.5px] whitespace-nowrap shrink-0">
          <button
            className="inline-flex items-center max-w-[160px] p-0 border-0 bg-transparent text-fg-dim cursor-pointer hover:text-fg truncate no-drag text-[11.5px]"
            onClick={() => { setFilter(""); setTreeVisible(true); setOpenDirs((prev) => new Set([...Array.from(prev), ""])); }}
            title={cwd}
          >
            {shortCwd(cwd) || t("workspace.title")}
          </button>
          {pathParts.map((part, index) => {
            const isLast = index === pathParts.length - 1;
            const dir = pathParts.slice(0, index + 1).join("/") + "/";
            return (
              <span className="inline-flex items-center min-w-0 gap-1.5" key={`${part}-${index}`}>
                <span className="text-fg-faint/40 select-none text-[10px] mx-px">›</span>
                <button
                  className={`inline-flex items-center max-w-[160px] p-0 border-0 bg-transparent cursor-pointer truncate no-drag text-[11.5px] ${
                    isLast ? "text-fg font-medium cursor-default" : "text-fg-dim hover:text-fg"
                  }`}
                  onClick={() => { if (isLast) return; setTreeVisible(true); setFilter(""); setOpenDirs((prev) => new Set([...Array.from(prev), ...breadcrumbDirs, dir])); void loadDir(dir); }}
                  title={isLast ? (selectedPath ?? undefined) : dir}
                >
                  {part}
                </button>
              </span>
            );
          })}
          {preview && preview.size > 0 && <span className="ml-auto shrink-0 font-mono text-[11px] text-fg-faint">{formatBytes(preview.size)}</span>}
        </div>

        <div className="flex-1 min-h-0 overflow-auto px-[18px] py-4">
          {!selectedPath ? (
            <div className="py-[18px] text-fg-faint text-[13px]">{t("workspace.pickFile")}</div>
          ) : loadingPreview ? (
            <div className="py-[18px] text-fg-faint text-[13px]">{t("workspace.loading")}</div>
          ) : preview?.err ? (
            <div className="py-[18px] text-err text-[13px]">{preview.err}</div>
          ) : preview?.binary ? (
            <div className="py-[18px] text-fg-faint text-[13px]">{t("workspace.binary")}</div>
          ) : preview ? (
            <>
              {preview.truncated && <div className="mb-2.5 px-2 py-1.5 border border-border-soft rounded-md bg-bg-soft text-fg-dim text-xs">{t("workspace.truncated")}</div>}
              {isMarkdown ? (
                <Markdown text={preview.body} />
              ) : (
                <CodeViewer value={preview.body || " "} language={languageFor(selectedPath)} />
              )}
            </>
          ) : null}
        </div>
      </section>}

      {treeVisible && previewVisible && (
        <button
          className="workspace-tree-resizer"
          type="button"
          role="separator"
          aria-orientation="vertical"
          aria-label={t("workspace.resizeTree")}
          aria-valuemin={WORKSPACE_TREE_MIN_WIDTH}
          aria-valuemax={WORKSPACE_TREE_MAX_WIDTH}
          aria-valuenow={effectiveTreeWidth}
          onPointerDown={startTreeResize}
          onKeyDown={resizeTreeWithKeyboard}
          onDoubleClick={() => setSavedTreeWidth(WORKSPACE_TREE_DEFAULT_WIDTH)}
          title={t("workspace.resizeTree")}
        />
      )}

      <section className="flex flex-col bg-bg-soft">
        <div className="flex items-center justify-end gap-1 h-[42px] px-2 py-[7px_8px_3px] shrink-0">
          <button
            className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-[color,background] duration-[0.12s] hover:text-fg hover:bg-bg-soft no-drag"
            onClick={hideTreeOrClosePanel}
            title={previewVisible ? t("workspace.hideTree") : t("workspace.close")}
          >
            <PanelRightClose size={15} />
          </button>
        </div>

        <div className="flex items-center gap-1.5 mx-2.5 my-1 mb-2 px-2 h-8 border border-border rounded-lg bg-bg text-fg-faint shrink-0">
          <Search size={14} />
          <input ref={filterRef} className="flex-1 min-w-0 border-0 outline-none bg-transparent text-fg text-[12.5px] placeholder:text-fg-faint" value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("workspace.filter")} />
        </div>
        <div className="flex-1 min-h-0 overflow-auto px-1.5 pb-3">
          {flattened
            ? flattened.map(({ path, entry }) => {
                const cleanPath = path.replace(/\/$/, "");
                const dir = parentPath(path);
                return (
                  <button
                    className={`w-full min-w-0 min-h-[38px] flex items-center gap-2 px-2 py-1.5 border-0 rounded-md bg-transparent text-fg-dim text-[12.5px] text-left cursor-pointer no-drag hover:bg-sidebar-hover hover:text-fg border-l-[3px] ${
                      selectedPath === path ? "!border-l-accent bg-sidebar-active !text-fg" : "border-l-transparent"
                    }`}
                    key={path}
                    onClick={() => (entry.isDir ? toggleDir(path) : selectFile(path))}
                    title={cleanPath}
                  >
                    {entry.isDir ? (
                      <Folder size={14} className={`shrink-0 ${selectedPath === path ? "text-accent" : "text-fg-dim"}`} />
                    ) : (
                      <FileText size={14} className={`shrink-0 ${selectedPath === path ? "text-accent" : "text-fg-faint"}`} />
                    )}
                    <span className="flex-1 min-w-0 flex flex-col gap-0.5 leading-[1.15]">
                      <span className={`min-w-0 truncate ${selectedPath === path ? "text-accent font-medium" : "text-fg"}`}>{basename(path)}</span>
                      {dir && <span className="min-w-0 truncate text-fg-faint text-[10.5px] font-mono">{dir}</span>}
                    </span>
                  </button>
                );
              })
            : renderRows("", 0)}
        </div>
      </section>
    </aside>
  );
}
