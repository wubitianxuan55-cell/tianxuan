import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronRight, Columns2, FileText, Folder, PanelRightClose, Search, X } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { DirEntry, FilePreview, WorkspaceChangeView } from "../lib/types";
import { CodeViewer } from "./CodeViewer";
import { UnifiedDiffView } from "./UnifiedDiffView";
import { Markdown } from "./Markdown";
import { Modal } from "./Modal";

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

function formatBytes(n: number): string {
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${Math.ceil(n / 1024)} KB`;
  return `${n} B`;
}

/**
 * WorkspacePanel — 文件/变更浏览面板。点击文件时预览以弹窗（Modal）打开，
 * 不再内嵌占用面板宽度（新交互设计）。
 */
export function WorkspacePanel({
  open,
  cwd,
  initialViewMode,
  onClose,
}: {
  open: boolean;
  cwd?: string;
  initialViewMode?: "files" | "changed";
  onClose: () => void;
}) {
  const t = useT();
  const filterRef = useRef<HTMLInputElement>(null);
  const [entriesByDir, setEntriesByDir] = useState<Record<string, DirEntry[]>>({});
  const [openDirs, setOpenDirs] = useState<Set<string>>(() => new Set([""]));
  const [filter, setFilter] = useState("");
  const [viewMode, setViewMode] = useState<"files" | "changed">(initialViewMode ?? "files");
  const [workspaceChanges, setWorkspaceChanges] = useState<WorkspaceChangeView[] | null>(null);

  // 弹窗预览状态
  const [modalPath, setModalPath] = useState<string | null>(null);
  const [modalPreview, setModalPreview] = useState<FilePreview | null>(null);
  const [modalLoading, setModalLoading] = useState(false);
  const [modalDiff, setModalDiff] = useState<string | null>(null);

  const loadDir = useCallback(async (dir: string) => {
    const entries = await app.ListDir(dir).catch(() => []);
    setEntriesByDir((prev) => ({ ...prev, [dir]: entries ?? [] }));
  }, []);

  const loadWorkspaceChanges = useCallback(async () => {
    const changes = await app.WorkspaceChanges().catch(() => []);
    setWorkspaceChanges(changes ?? []);
  }, []);

  // 打开文件 → 弹窗预览
  const openFilePreview = useCallback((path: string, diff?: string) => {
    setFilter("");
    const dirs = parentPath(path);
    if (dirs) {
      setOpenDirs((prev) => new Set([...Array.from(prev), dirs]));
    }
    setModalDiff(diff ?? null);
    setModalPath(path);
  }, []);

  useEffect(() => {
    if (!open) return;
    setEntriesByDir({});
    setOpenDirs(new Set([""]));
    setFilter("");
    setModalPath(null);
    setModalPreview(null);
    setModalDiff(null);
    void loadDir("");
    if (viewMode === "changed" && workspaceChanges === null) {
      void loadWorkspaceChanges();
    }
  }, [cwd, loadDir, open]);

  // auto-load workspace changes when switching to changed view
  useEffect(() => {
    if (open && viewMode === "changed" && workspaceChanges === null) {
      void loadWorkspaceChanges();
    }
  }, [viewMode, open, workspaceChanges, loadWorkspaceChanges]);

  // 加载弹窗预览内容
  useEffect(() => {
    if (!modalPath) return;
    let live = true;
    setModalLoading(true);
    setModalPreview(null);
    app
      .ReadFile(modalPath)
      .then((next) => {
        if (live) setModalPreview(next);
      })
      .catch((err) => {
        if (live) {
          setModalPreview({
            path: modalPath,
            body: "",
            size: 0,
            truncated: false,
            binary: false,
            err: String(err?.message ?? err),
          });
        }
      })
      .finally(() => {
        if (live) setModalLoading(false);
      });
    return () => {
      live = false;
    };
  }, [modalPath]);

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

  if (!open) return null;

  const renderRows = (dir: string, depth: number): JSX.Element[] => {
    const entries = entriesByDir[dir] ?? [];
    return entries.flatMap((entry) => {
      const path = entryPath(dir, entry);
      const isOpen = openDirs.has(path);
      const row = (
        <button
          className={`w-full min-w-0 h-[30px] flex items-center gap-1.5 border-0 rounded-md bg-transparent text-fg-dim text-[12.5px] text-left cursor-pointer no-drag hover:bg-sidebar-hover hover:text-fg border-l-[3px] border-l-transparent`}
          key={path}
          onClick={() => (entry.isDir ? toggleDir(path) : openFilePreview(path))}
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
            <Folder size={14} className={`shrink-0 ${isOpen ? "text-accent" : "text-fg-dim"}`} />
          ) : (
            <FileText size={14} className="shrink-0 text-fg-faint" />
          )}
          <span className="min-w-0 truncate">{entry.name}</span>
        </button>
      );
      if (!entry.isDir || !isOpen) return [row];
      return [row, ...renderRows(path, depth + 1)];
    });
  };

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

  const isModalMarkdown = modalPath?.toLowerCase().endsWith(".md") ?? false;

  return (
    <>
      <aside
        className="workspace-panel"
        aria-label={t("workspace.title")}
      >
        <div className="flex items-center justify-end gap-1 h-[42px] px-2 py-[7px_8px_3px] shrink-0">
          <button
            className={`inline-flex items-center justify-center h-7 border-0 rounded-md bg-transparent cursor-pointer transition-[color,background] duration-[var(--dur-fast)] no-drag px-2 text-[11.5px] font-medium gap-1 ${viewMode === "files" ? "text-accent bg-accent-soft" : "text-fg-dim hover:text-fg hover:bg-bg-soft"}`}
            onClick={() => { setViewMode("files"); void loadDir(""); }}
          >
            <Folder size={13} /> 文件
          </button>
          <button
            className={`inline-flex items-center justify-center h-7 border-0 rounded-md bg-transparent cursor-pointer transition-[color,background] duration-[var(--dur-fast)] no-drag px-2 text-[11.5px] font-medium gap-1 ${viewMode === "changed" ? "text-accent bg-accent-soft" : "text-fg-dim hover:text-fg hover:bg-bg-soft"}`}
            onClick={() => { setViewMode("changed"); void loadWorkspaceChanges(); }}
          >
            <Columns2 size={13} /> 变更
          </button>
          <div className="flex-1" />
          <button
            className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-[color,background] duration-[var(--dur-fast)] hover:text-fg hover:bg-bg-soft no-drag"
            onClick={onClose}
            title={t("workspace.close")}
          >
            <PanelRightClose size={15} />
          </button>
        </div>

        {viewMode === "files" && (
          <div className="flex items-center gap-1.5 mx-2.5 my-1 mb-2 px-2 h-8 border border-border rounded-lg bg-bg text-fg-faint shrink-0">
            <Search size={14} />
            <input ref={filterRef} className="flex-1 min-w-0 border-0 outline-none bg-transparent text-fg text-[12.5px] placeholder:text-fg-faint" value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("workspace.filter")} />
          </div>
        )}
        <div className="flex-1 min-h-0 overflow-auto px-1.5 pb-3">
          {viewMode === "changed" ? (
            workspaceChanges === null ? (
              <div className="flex flex-col items-center justify-center gap-3 py-12">
                <div className="w-6 h-6 rounded-full border-2 border-accent/30 border-t-accent animate-spin" />
                <span className="text-fg-faint text-[12px]">加载变更中…</span>
              </div>
            ) : workspaceChanges.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-2 py-12">
                <FileText size={28} className="text-fg-faint/30" />
                <span className="text-fg-faint text-[12.5px]">本会话暂无文件变更</span>
                <span className="text-fg-faint/40 text-[11px]">编辑文件后变更会自动出现在这里</span>
              </div>
            ) : (
              <div>
                <div className="flex items-center gap-3 px-2 py-1.5 mb-1 text-[11px] text-fg-faint">
                  <span className="font-medium">{workspaceChanges.length} 个文件</span>
                  <span className="text-border/40">·</span>
                  <span className="text-ok">+{workspaceChanges.reduce((s, c) => s + c.added, 0)}</span>
                  <span className="text-err">-{workspaceChanges.reduce((s, c) => s + c.removed, 0)}</span>
                </div>
                {workspaceChanges.map((ch) => {
                  const dir = ch.path.includes("/") ? ch.path.slice(0, ch.path.lastIndexOf("/")) : "";
                  return (
                    <button
                      className="w-full min-w-0 min-h-[38px] flex items-center gap-2 px-2 py-1.5 border-0 rounded-md bg-transparent text-fg-dim text-[12.5px] text-left cursor-pointer no-drag hover:bg-sidebar-hover hover:text-fg border-l-[3px] border-l-transparent"
                      key={ch.path}
                      onClick={() => openFilePreview(ch.path, ch.diff)}
                      title={ch.path}
                    >
                      <FileText size={14} className="shrink-0 text-fg-faint" />
                      <span className="flex-1 min-w-0 flex flex-col gap-0.5 leading-[1.15]">
                        <span className="min-w-0 truncate text-fg">{ch.path.split("/").pop()}</span>
                        {dir && <span className="min-w-0 truncate text-fg-faint text-[10.5px] font-mono">{dir}</span>}
                      </span>
                      <span className="flex items-center gap-1.5 shrink-0 text-[10.5px] tabular-nums font-mono">
                        {ch.added > 0 && <span className="text-ok">+{ch.added}</span>}
                        {ch.removed > 0 && <span className="text-err">-{ch.removed}</span>}
                      </span>
                    </button>
                  );
                })}
              </div>
            )
          ) : (
            flattened
              ? flattened.map(({ path, entry }) => {
                  const cleanPath = path.replace(/\/$/, "");
                  const dir = parentPath(path);
                  return (
                    <button
                      className="w-full min-w-0 min-h-[38px] flex items-center gap-2 px-2 py-1.5 border-0 rounded-md bg-transparent text-fg-dim text-[12.5px] text-left cursor-pointer no-drag hover:bg-sidebar-hover hover:text-fg border-l-[3px] border-l-transparent"
                      key={path}
                      onClick={() => (entry.isDir ? toggleDir(path) : openFilePreview(cleanPath))}
                      title={cleanPath}
                    >
                      {entry.isDir ? (
                        <Folder size={14} className="shrink-0 text-fg-dim" />
                      ) : (
                        <FileText size={14} className="shrink-0 text-fg-faint" />
                      )}
                      <span className="flex-1 min-w-0 flex flex-col gap-0.5 leading-[1.15]">
                        <span className="min-w-0 truncate text-fg">{basename(path)}</span>
                        {dir && <span className="min-w-0 truncate text-fg-faint text-[10.5px] font-mono">{dir}</span>}
                      </span>
                    </button>
                  );
                })
              : renderRows("", 0)
          )}
        </div>
      </aside>

      {/* ── 文件预览弹窗 ── */}
      {modalPath && (
        <Modal onClose={() => { setModalPath(null); setModalDiff(null); }} wide>
          <div className="flex items-center gap-2.5 px-4 py-2.5 border-b border-border-soft shrink-0">
            <FileText size={15} className="shrink-0 text-fg-faint" />
            <span className="font-medium text-[13px] truncate">{basename(modalPath)}</span>
            <span className="text-fg-faint font-mono text-[11px] truncate">{modalPath}</span>
            {modalPreview && modalPreview.size > 0 && (
              <span className="ml-auto shrink-0 font-mono text-[11px] text-fg-faint">{formatBytes(modalPreview.size)}</span>
            )}
            <button
              className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:text-fg hover:bg-bg-soft shrink-0"
              onClick={() => { setModalPath(null); setModalDiff(null); }}
              title={t("workspace.close")}
            >
              <X size={15} />
            </button>
          </div>
          <div className="flex-1 min-h-0">
            {modalDiff ? (
              <UnifiedDiffView diff={modalDiff} />
            ) : (
              <div className="flex-1 min-h-0 overflow-auto px-4 py-3">
                {modalLoading ? (
                  <div className="py-10 text-center text-fg-faint text-[13px]">{t("workspace.loading")}</div>
                ) : modalPreview?.err ? (
                  <div className="py-6 text-err text-[13px]">{modalPreview.err}</div>
                ) : modalPreview?.binary ? (
                  <div className="py-6 text-fg-faint text-[13px]">{t("workspace.binary")}</div>
                ) : modalPreview ? (
                  <>
                    {modalPreview.truncated && <div className="mb-2.5 px-2 py-1.5 border border-border-soft rounded-md bg-bg-soft text-fg-dim text-xs">{t("workspace.truncated")}</div>}
                    {isModalMarkdown ? (
                      <Markdown text={modalPreview.body} />
                    ) : (
                      <CodeViewer value={modalPreview.body || " "} language={languageFor(modalPath)} />
                    )}
                  </>
                ) : null}
              </div>
            )}
          </div>
        </Modal>
      )}
    </>
  );
}
