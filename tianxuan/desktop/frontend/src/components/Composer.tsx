import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, ClipboardEvent, DragEvent, KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import { ArrowUp, Check, ChevronDown, Clock, Eye, FileText, FolderGit2, FolderPlus, Search, Square, Trash2, X } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { clearLayoutSize, loadOptionalLayoutSize, saveLayoutSize } from "../lib/layoutPreferences";
import type { CommandInfo, DirEntry, SlashArgItem, SlashArgsResult, WorkspaceView } from "../lib/types";
import { useStore } from "../lib/store";
import { SlashMenu } from "./SlashMenu";
import { ArgMenu } from "./ArgMenu";
import { FileMenu } from "./FileMenu";

interface Attachment { path: string; previewUrl: string; }

const LONG_PASTE_MIN_CHARS = 2000;
const LONG_PASTE_MIN_LINES = 20;
const COMPOSER_MIN_HEIGHT = 86;
const COMPOSER_MAX_HEIGHT = 360;
const COMPOSER_MAX_VIEWPORT_RATIO = 0.4;
const INPUT_HISTORY_KEY = "reasonix.inputHistory";
const MAX_INPUT_HISTORY = 50;

type PastedBlock = { label: string; text: string };

function lineCount(s: string): number {
  if (s === "") return 0;
  return s.split(/\r\n|\r|\n/).length;
}

function shouldFoldPaste(s: string): boolean {
  return s.length >= LONG_PASTE_MIN_CHARS || lineCount(s) >= LONG_PASTE_MIN_LINES;
}

function renderPastedBlock(block: PastedBlock): string {
  return `${block.label}\n\n--- Begin ${block.label} ---\n${block.text}\n--- End ${block.label} ---`;
}

function useDebounce<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => { const t = setTimeout(() => setDebounced(value), delay); return () => clearTimeout(t); }, [value, delay]);
  return debounced;
}

function composerMaxHeight(): number {
  if (typeof window === "undefined") return COMPOSER_MAX_HEIGHT;
  return Math.max(COMPOSER_MIN_HEIGHT, Math.min(COMPOSER_MAX_HEIGHT, Math.floor(window.innerHeight * COMPOSER_MAX_VIEWPORT_RATIO)));
}
function clampComposerHeight(h: number): number {
  return Math.min(Math.max(Math.round(h), COMPOSER_MIN_HEIGHT), composerMaxHeight());
}
function loadComposerHeight(): number | null {
  return loadOptionalLayoutSize("composerHeight", clampComposerHeight);
}

export function Composer({
  running, cwd, onSend, onCancel, agentMode, onSetAgentMode, yolo, onToggleYolo, onPickFolder, disabled,
}: {
  running: boolean; cwd?: string;
  onSend: (displayText: string, submitText?: string) => void;
  onCancel: () => string | undefined; agentMode?: string; onSetAgentMode?: (m: "explore" | "develop" | "orchestrate") => void;
  yolo?: boolean; onToggleYolo?: () => void;
  onPickFolder: (path?: string) => Promise<string>; disabled?: boolean;
}) {
  const t = useT();
  const [text, setText] = useState("");
  const debouncedText = useDebounce(text, 80);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [pastedBlocks, setPastedBlocks] = useState<PastedBlock[]>([]);
  const [openPastedLabels, setOpenPastedLabels] = useState<string[]>([]);
  const [pendingPaste, setPendingPaste] = useState(0);
  const pastedBlocksRef = useRef<PastedBlock[]>([]);
  const nextPasteId = useRef(1);
  const [active, setActive] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [workspaceMenuOpen, setWorkspaceMenuOpen] = useState(false);
  const [workspaceQuery, setWorkspaceQuery] = useState("");
  const [workspaces, setWorkspaces] = useState<WorkspaceView[]>([]);
  const [composerHeight, setComposerHeight] = useState<number | null>(loadComposerHeight);
  const [composerResizing, setComposerResizing] = useState(false);
  const taRef = useRef<HTMLTextAreaElement>(null);
  const composerCardRef = useRef<HTMLDivElement>(null);
  const workspaceAnchorRef = useRef<HTMLDivElement>(null);
  const workspaceMenuRef = useRef<HTMLDivElement>(null);
  const wasRunning = useRef(running);

  // 排队
  const queueRef = useRef<string[]>([]);
  const [queueLen, setQueueLen] = useState(0);
  const onSendRef = useRef(onSend);
  onSendRef.current = onSend;
  useEffect(() => {
    if (!running && queueRef.current.length > 0) {
      const timer = setTimeout(() => {
        const next = queueRef.current.shift()!;
        setQueueLen(queueRef.current.length);
        onSendRef.current(next, next);
      }, 50);
      return () => clearTimeout(timer);
    }
  }, [running]);

  // 计时
  const turnStartAt = useStore(useCallback((s) => s.turnStartAt, []));
  const turnActive = useStore(useCallback((s) => s.turnActive, []));
  const turnTokens = useStore(useCallback((s) => s.turnTokens, []));
  const [elapsed, setElapsed] = useState(0);
  const [finalElapsed, setFinalElapsed] = useState<number | null>(null);
  useEffect(() => {
    if (!turnActive) { if (turnStartAt > 0) setFinalElapsed((Date.now() - turnStartAt) / 1000); return; }
    setFinalElapsed(null);
    const tick = () => setElapsed((Date.now() - turnStartAt) / 1000);
    tick();
    const id = setInterval(tick, 200);
    return () => clearInterval(id);
  }, [turnActive, turnStartAt]);
  useEffect(() => {
    if (wasRunning.current && !running && text.trim() === "") {
      pastedBlocksRef.current = [];
      setPastedBlocks([]); setOpenPastedLabels([]);
    }
    wasRunning.current = running;
  }, [running, text]);

  // ── / 命令 ──
  const [commands, setCommands] = useState<CommandInfo[]>([]);
  useEffect(() => { app.Commands().then(setCommands).catch(() => {}); }, []);
  const slashQuery = useMemo(() => (!text.startsWith("/") || /\s/.test(text) ? null : text.slice(1).toLowerCase()), [text]);
  const slashMatches = useMemo(() => (slashQuery === null ? [] : commands.filter((c) => c.name.toLowerCase().includes(slashQuery)).slice(0, 8)), [slashQuery, commands]);

  // ── 命令参数 ──
  const [argRes, setArgRes] = useState<SlashArgsResult | null>(null);
  useEffect(() => {
    if (!text.startsWith("/") || !/\s/.test(text)) { setArgRes(null); return; }
    let live = true;
    app.SlashArgs(text).then((r) => {
      if (!live) return;
      const useful = (r.items ?? []).filter((it) => text.slice(0, r.from) + it.insert !== text);
      setArgRes(useful.length > 0 ? { items: useful, from: r.from } : null); setActive(0);
    }).catch(() => {});
    return () => { live = false; };
  }, [text]);

  // ── @ 文件引用 ──
  const atRaw = useMemo(() => { const m = /(?:^|\s)@([^\s]*)$/.exec(debouncedText); return m ? m[1] : null; }, [debouncedText]);
  const atDir = useMemo(() => { if (atRaw === null) return ""; const s = atRaw.lastIndexOf("/"); return s >= 0 ? atRaw.slice(0, s + 1) : ""; }, [atRaw]);
  const atFrag = useMemo(() => { if (atRaw === null) return ""; const s = atRaw.lastIndexOf("/"); return (s >= 0 ? atRaw.slice(s + 1) : atRaw).toLowerCase(); }, [atRaw]);
  const [entries, setEntries] = useState<DirEntry[]>([]);
  const dirCache = useRef<Record<string, DirEntry[]>>({});
  useEffect(() => {
    if (atRaw === null) return;
    const cached = dirCache.current[atDir];
    if (cached) { setEntries(cached); return; }
    let live = true;
    app.ListDir(atDir).then((es) => { const list = es ?? []; dirCache.current[atDir] = list; if (live) setEntries(list); }).catch(() => {});
    return () => { live = false; };
  }, [atRaw === null, atDir]);
  const atMatches = useMemo(() => (atRaw === null ? [] : entries.filter((e) => e.name.toLowerCase().includes(atFrag)).slice(0, 10)), [atRaw, atFrag, entries]);

  // ── 菜单状态 ──
  const menuMode: "slash" | "slasharg" | "at" | null =
    slashMatches.length > 0 && !dismissed ? "slash"
    : argRes && argRes.items.length > 0 && !dismissed ? "slasharg"
    : atMatches.length > 0 && !dismissed ? "at"
    : null;
  const menuCount = menuMode === "slash" ? slashMatches.length : menuMode === "slasharg" ? argRes!.items.length : menuMode === "at" ? atMatches.length : 0;
  useEffect(() => { setActive(0); setDismissed(false); }, [slashQuery, atRaw]);

  const setTextCaretEnd = (next: string) => {
    setText(next);
    requestAnimationFrame(() => { const ta = taRef.current; if (ta) { ta.focus(); ta.selectionStart = ta.selectionEnd = next.length; } });
  };

  const expandPastedBlocks = (displayText: string): string => {
    let expanded = displayText;
    for (const b of pastedBlocksRef.current) { if (expanded.includes(b.label)) expanded = expanded.split(b.label).join(renderPastedBlock(b)); }
    return expanded;
  };

  const submit = () => {
    if (disabled) return;
    const tTrim = text.trim();
    if ((!tTrim && attachments.length === 0) || pendingPaste > 0) return;
    const refs = attachments.map((a) => `@${a.path}`).join(" ");
    const displayText = [tTrim, refs].filter(Boolean).join(tTrim && refs ? " " : "");
    const submitText = [expandPastedBlocks(tTrim), refs].filter(Boolean).join(tTrim && refs ? " " : "");
    if (displayText.trim()) {
      try {
        const history = JSON.parse(sessionStorage.getItem(INPUT_HISTORY_KEY) || "[]") as string[];
        history.unshift(displayText); sessionStorage.setItem(INPUT_HISTORY_KEY, JSON.stringify(history.slice(0, MAX_INPUT_HISTORY)));
      } catch {}
    }
    setHistoryIndex(-1);
    if (running) { queueRef.current.push(submitText); setQueueLen(queueRef.current.length); setText(""); setAttachments([]); return; }
    onSend(displayText, submitText); setText(""); setAttachments([]);
  };

  const attachImageFiles = async (files: File[]) => {
    const images = files.filter((f) => f.type.startsWith("image/"));
    if (images.length === 0) return;
    for (const file of images) {
      setPendingPaste((n) => n + 1);
      try {
        const dataUrl = await new Promise<string>((res, rej) => { const r = new FileReader(); r.onload = () => res(String(r.result)); r.onerror = () => rej(r.error); r.readAsDataURL(file); });
        const path = await app.SavePastedImage(dataUrl);
        const previewUrl = await app.AttachmentDataURL(path);
        setAttachments((prev) => [...prev, { path, previewUrl }]);
      } catch {} finally { setPendingPaste((n) => Math.max(0, n - 1)); }
    }
  };

  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(e.clipboardData.files).filter((f) => f.type.startsWith("image/"));
    if (files.length > 0) { e.preventDefault(); void attachImageFiles(files); return; }
    // Detect screenshot/image in clipboard (no file object, just image data).
    const hasImageData = Array.from(e.clipboardData.items).some(
      (it) => it.type.startsWith("image/")
    );
    if (hasImageData) {
      // Notify: screenshot paste is detected but needs Go-side SaveClipboardImage.
      // The user can drag-drop the image or save it as a file first.
      e.preventDefault();
      return;
    }
    const pasted = e.clipboardData.getData("text");
    if (!shouldFoldPaste(pasted)) return;
    e.preventDefault();
    const ta = e.currentTarget;
    const start = ta.selectionStart ?? text.length;
    const end = ta.selectionEnd ?? text.length;
    const id = nextPasteId.current++;
    const label = t("composer.pastedLabel", { id, lines: lineCount(pasted) });
    const block: PastedBlock = { label, text: pasted };
    const next = text.slice(0, start) + label + text.slice(end);
    pastedBlocksRef.current = [...pastedBlocksRef.current, block];
    setPastedBlocks((prev) => [...prev, block]); setText(next);
    requestAnimationFrame(() => { const n = taRef.current; if (n) { n.focus(); n.selectionStart = n.selectionEnd = start + label.length; } });
  };

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    const files = Array.from(e.dataTransfer.files);
    if (!files.some((f) => f.type.startsWith("image/"))) return;
    e.preventDefault(); setDragOver(false); void attachImageFiles(files);
  };
  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    if (!Array.from(e.dataTransfer.items).some((it) => it.kind === "file")) return;
    e.preventDefault(); setDragOver(true);
  };
  const onDragLeave = () => setDragOver(false);

  const handleCancel = () => { queueRef.current = []; setQueueLen(0); const restored = onCancel(); if (typeof restored === "string") setTextCaretEnd(restored); };

  const pickCommand = (c: CommandInfo) => setTextCaretEnd("/" + c.name + " ");
  const pickEntry = (e: DirEntry) => {
    const atPos = text.length - (atRaw?.length ?? 0) - 1;
    const prefix = text.slice(0, atPos);
    setTextCaretEnd(prefix + "@" + atDir + e.name + (e.isDir ? "/" : " "));
  };
  const pickArg = (it: SlashArgItem) => { if (!argRes) return; setTextCaretEnd(text.slice(0, argRes.from) + it.insert); };
  const pickActive = () => {
    if (menuMode === "slash") pickCommand(slashMatches[active]);
    else if (menuMode === "slasharg" && argRes) pickArg(argRes.items[active]);
    else if (menuMode === "at") pickEntry(atMatches[active]);
  };

  const activePastedBlocks = pastedBlocks.filter((b) => text.includes(b.label));
  const togglePastedPreview = (label: string) => setOpenPastedLabels((p) => p.includes(label) ? p.filter((x) => x !== label) : [...p, label]);
  const removePastedBlock = (b: PastedBlock) => { pastedBlocksRef.current = pastedBlocksRef.current.filter((x) => x.label !== b.label); setPastedBlocks((p) => p.filter((x) => x.label !== b.label)); setOpenPastedLabels((p) => p.filter((x) => x !== b.label)); setTextCaretEnd(text.split(b.label).join("")); };
  const expandPastedBlock = (b: PastedBlock) => { pastedBlocksRef.current = pastedBlocksRef.current.filter((x) => x.label !== b.label); setPastedBlocks((p) => p.filter((x) => x.label !== b.label)); setOpenPastedLabels((p) => p.filter((x) => x !== b.label)); setTextCaretEnd(text.split(b.label).join(b.text)); };

  // ── 工作区菜单 ──
  const workspaceName = useMemo(() => { if (!cwd) return ""; const parts = cwd.split(/[/\\]/).filter(Boolean); return parts.length > 0 ? parts[parts.length - 1] : cwd; }, [cwd]);
  const loadWorkspaces = () => { app.ListWorkspaces().then(setWorkspaces).catch(() => setWorkspaces([])); };
  useEffect(() => { if (workspaceMenuOpen) loadWorkspaces(); }, [workspaceMenuOpen, cwd]);
  useEffect(() => {
    if (!workspaceMenuOpen) return;
    const close = (e: MouseEvent) => { const tgt = e.target as Node; if (workspaceAnchorRef.current?.contains(tgt) || workspaceMenuRef.current?.contains(tgt)) return; setWorkspaceMenuOpen(false); };
    document.addEventListener("mousedown", close); return () => document.removeEventListener("mousedown", close);
  }, [workspaceMenuOpen]);
  const filteredWorkspaces = useMemo(() => { const q = workspaceQuery.trim().toLowerCase(); if (!q) return workspaces; return workspaces.filter((w) => `${w.name} ${w.path}`.toLowerCase().includes(q)); }, [workspaceQuery, workspaces]);
  const chooseWorkspace = async (path?: string) => { const next = await onPickFolder(path); if (next) { setWorkspaceMenuOpen(false); setWorkspaceQuery(""); } };

  useEffect(() => {
    const onResize = () => setComposerHeight((h) => (h === null ? null : clampComposerHeight(h)));
    window.addEventListener("resize", onResize); return () => window.removeEventListener("resize", onResize);
  }, []);
  const saveComposerHeight = (h: number) => saveLayoutSize("composerHeight", h, clampComposerHeight);
  const resetComposerHeight = () => { setComposerHeight(null); clearLayoutSize("composerHeight"); };
  const onComposerResizeStart = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    const card = composerCardRef.current; if (!card) return;
    e.preventDefault();
    const startY = e.clientY;
    const startHeight = composerHeight ?? card.getBoundingClientRect().height;
    let nextHeight = clampComposerHeight(startHeight);
    let moved = false;
    setComposerResizing(true); document.body.classList.add("composer-resizing");
    const onMove = (ev: PointerEvent) => { moved = true; nextHeight = clampComposerHeight(startHeight + startY - ev.clientY); setComposerHeight(nextHeight); };
    const onUp = () => { setComposerResizing(false); document.body.classList.remove("composer-resizing"); if (moved) saveComposerHeight(nextHeight); document.removeEventListener("pointermove", onMove); document.removeEventListener("pointerup", onUp); document.removeEventListener("pointercancel", onUp); };
    document.addEventListener("pointermove", onMove); document.addEventListener("pointerup", onUp); document.addEventListener("pointercancel", onUp);
  };

  // ── 键盘处理 ──
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    const composing = e.nativeEvent.isComposing;
    if (menuMode && !composing) {
      if (e.key === "ArrowDown") { e.preventDefault(); setActive((i) => (i + 1) % menuCount); return; }
      if (e.key === "ArrowUp") { e.preventDefault(); setActive((i) => (i - 1 + menuCount) % menuCount); return; }
      if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pickActive(); return; }
      if (e.key === "Escape") { e.preventDefault(); setDismissed(true); return; }
    }
    if (!menuMode && !composing) {
      if (e.key === "ArrowUp" && text === "") { e.preventDefault(); navigateHistory(1); return; }
      if (e.key === "ArrowDown" && historyIndex >= 0) { e.preventDefault(); navigateHistory(-1); return; }
      if (e.key !== "ArrowUp" && e.key !== "ArrowDown" && historyIndex >= 0) setHistoryIndex(-1);
    }
    if (e.key === "Enter" && !e.shiftKey && !composing) { e.preventDefault(); submit(); }
    if (e.key === "Escape" && running) { e.preventDefault(); handleCancel(); }
  };

  // ── 历史 ──
  const [historyIndex, setHistoryIndex] = useState(-1);
  const historyDraft = useRef("");
  const navigateHistory = (dir: 1 | -1) => {
    try {
      const history: string[] = JSON.parse(sessionStorage.getItem(INPUT_HISTORY_KEY) || "[]");
      if (history.length === 0) return;
      if (historyIndex === -1) historyDraft.current = text;
      const next = Math.max(-1, Math.min(history.length - 1, historyIndex + dir));
      setHistoryIndex(next); setText(next === -1 ? historyDraft.current : history[next] || "");
    } catch {}
  };

  const composerCardStyle = composerHeight === null ? undefined : ({ "--composer-height": `${composerHeight}px` } as CSSProperties);
  const displayElapsed = finalElapsed ?? elapsed;

  // ── 项目感知 placeholder ──
  const placeholderText = useMemo(() => {
    if (disabled) return t("common.loading");
    if (running && queueLen > 0) return `排队中 (${queueLen})…`;
    if (running) return t("composer.placeholderRunning");
    if (cwd && workspaceName) return `在 ${workspaceName}/ 中提问…`;
    return t("composer.placeholder");
  }, [disabled, running, queueLen, cwd, workspaceName, t]);

  return (
    <div className="relative max-w-[--maxw] mx-auto">
      {/* ── 计时条 ── */}
      {(turnActive || finalElapsed !== null) && (
        <div className="flex items-center gap-1.5 pb-1.5 pl-1 text-fg-faint text-[11px] tabular-nums font-mono">
          <Clock size={11} className="text-accent" />
          <span>{displayElapsed.toFixed(1)}s</span>
          {turnTokens > 0 && <span className="text-fg-faint/70">{turnTokens} tok</span>}
        </div>
      )}

      {/* ── 工作区切换菜单 ── */}
      {workspaceMenuOpen && cwd && (
        <div
          className="absolute left-2.5 bottom-12 z-40 w-[min(320px,82vw)] p-2.5 border border-border rounded-xl bg-bg-elev anim-menu-in no-drag"
          style={{boxShadow: "var(--ds-shadow-dropdown)"}}
          ref={workspaceMenuRef}
        >
          <label className="flex items-center gap-[7px] px-2 py-1.5 mb-1 border border-border-soft rounded-md bg-bg-soft focus-within:border-accent transition-colors">
            <Search size={14} className="text-fg-faint" />
            <input autoFocus className="flex-1 border-0 bg-transparent text-fg text-[13px] outline-none placeholder:text-fg-faint"
              value={workspaceQuery} onChange={(e) => setWorkspaceQuery(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Escape") setWorkspaceMenuOpen(false); }}
              placeholder={t("composer.searchProjects")} />
          </label>
          <div className="max-h-[280px] overflow-y-auto mb-1">
            {filteredWorkspaces.map((w) => (
              <button key={w.path}
                className={`flex items-center gap-2.5 w-full px-2 py-1.5 bg-transparent border-0 rounded-lg text-left cursor-pointer transition-colors duration-100 ${w.current ? "text-accent bg-accent-soft font-medium" : "text-fg-dim hover:bg-bg-soft hover:text-fg"}`}
                onClick={() => { if (w.current) { setWorkspaceMenuOpen(false); return; } void chooseWorkspace(w.path); }}
                title={w.path}>
                <FolderGit2 size={15} className="shrink-0" />
                <span className="min-w-0 truncate flex-1 text-[13px]">{w.name}</span>
                {w.current && <Check size={15} className="text-accent shrink-0" />}
              </button>
            ))}
            {filteredWorkspaces.length === 0 && <div className="py-4 text-fg-faint text-xs text-center">{t("composer.noProjectMatches")}</div>}
          </div>
          <div className="pt-1 border-t border-border-soft">
            <button className="flex items-center gap-2.5 w-full px-2 py-1.5 bg-transparent border-0 rounded-lg text-left cursor-pointer text-fg-dim hover:bg-bg-soft hover:text-fg text-[13px] transition-colors" onClick={() => void chooseWorkspace()}>
              <FolderPlus size={15} className="shrink-0" />
              <span>{t("composer.addProject")}</span>
            </button>
          </div>
        </div>
      )}

      {/* ── 菜单（命令/参数/文件）── */}
      {menuMode === "slash" && <SlashMenu items={slashMatches} activeIndex={active} onPick={pickCommand} onHover={setActive} />}
      {menuMode === "slasharg" && argRes && <ArgMenu items={argRes.items} activeIndex={active} onPick={pickArg} onHover={setActive} />}
      {menuMode === "at" && <FileMenu items={atMatches} activeIndex={active} onPick={pickEntry} onHover={setActive} />}

      {/* ── 附件预览 ── */}
      {attachments.length > 0 && (
        <div className="flex flex-wrap gap-1.5 px-1 pb-1.5">
          {attachments.map((a) => (
            <div className="flex items-center gap-1.5 pl-1.5 pr-1 py-1 bg-bg-elev-2 border border-border-soft rounded-lg text-xs" key={a.path}>
              <img src={a.previewUrl} alt="" className="w-8 h-8 rounded object-cover" />
              <span className="max-w-[120px] truncate text-fg-dim font-mono text-[11px]">{a.path.split("/").pop()}</span>
              <button type="button" className="flex items-center justify-center w-5 h-5 bg-transparent border-0 rounded text-fg-faint cursor-pointer hover:text-err hover:bg-bg-soft transition-colors" title="移除" onClick={() => setAttachments((prev) => prev.filter((x) => x.path !== a.path))}><X size={13} /></button>
            </div>
          ))}
        </div>
      )}

      {/* ── 粘贴块 ── */}
      {activePastedBlocks.length > 0 && (
        <div className="pb-1.5">
          {activePastedBlocks.map((block) => {
            const open = openPastedLabels.includes(block.label);
            return (
              <div className="mb-1 border border-border-soft rounded-lg overflow-hidden" key={block.label}>
                <div className="flex items-center gap-1.5 px-2 py-1 text-xs">
                  <FileText size={14} className="text-fg-faint shrink-0" />
                  <span className="font-mono text-xs text-fg-dim min-w-0 truncate">{block.label}</span>
                  <div className="flex items-center gap-0.5 ml-auto">
                    <ActionBtn title={open ? "收起预览" : "展开预览"} onClick={() => togglePastedPreview(block.label)}><Eye size={13} /></ActionBtn>
                    <ActionBtn title="展开内容" onClick={() => expandPastedBlock(block)}><span className="text-[11px]">展开</span></ActionBtn>
                    <ActionBtn title="移除" danger onClick={() => removePastedBlock(block)}><Trash2 size={13} /></ActionBtn>
                  </div>
                </div>
                {open && <pre className="m-0 p-2 bg-bg text-fg-dim text-xs leading-relaxed whitespace-pre-wrap break-words max-h-[140px] overflow-y-auto border-t border-border-soft">{block.text}</pre>}
              </div>
            );
          })}
        </div>
      )}

      {/* ── 输入卡片 ── */}
      <div
        className={`relative border border-border-soft bg-bg-elev rounded-2xl overflow-hidden transition-[border-color,box-shadow] duration-[var(--dur-base)] focus-within:border-accent/30 focus-within:shadow-[0_0_0_1px_var(--accent-soft),var(--ds-shadow-composer)] ${composerHeight !== null ? "flex flex-col" : ""} ${composerResizing ? "cursor-ns-resize" : ""}`}
        style={{ ...(composerHeight !== null ? { height: "var(--composer-height)" } : {}), ...composerCardStyle }}
        ref={composerCardRef}
      >
        {/* 拖拽调整大小把手 */}
        <div
          className="absolute top-0 left-[14px] right-[14px] z-[5] h-2 cursor-ns-resize no-drag touch-none"
          onPointerDown={onComposerResizeStart}
          onDoubleClick={resetComposerHeight}
        />

        {/* 主输入行 */}
        <div
          className={`flex gap-2 items-center shrink-0 min-h-0 bg-transparent border-0 border-b border-border-soft rounded-none px-[13px] py-2.5 ${composerHeight !== null ? "flex-1 items-start" : ""} ${dragOver ? "outline outline-1 outline-dashed outline-accent outline-offset-[-4px] bg-accent-[0.02]" : ""} ${disabled ? "opacity-50 pointer-events-none" : ""}`}
          onDrop={onDrop} onDragOver={onDragOver} onDragLeave={onDragLeave}
        >
          <span className="text-accent font-mono font-semibold text-lg leading-[1.55] shrink-0 select-none">›</span>
          <textarea
            ref={taRef}
            className={`flex-1 resize-none border-0 bg-transparent text-fg leading-[1.55] max-h-[200px] outline-none placeholder:text-fg-faint ${composerHeight !== null ? "h-full max-h-none overflow-y-auto" : ""}`}
            style={{ fieldSizing: "content" }}
            value={text} onChange={(e) => setText(e.target.value)}
            onPaste={onPaste} onKeyDown={onKeyDown}
            placeholder={placeholderText}
            rows={1} disabled={disabled}
          />
          {running && (
            <button className="inline-flex items-center justify-center w-[30px] h-[30px] border-0 rounded-md cursor-pointer shrink-0 transition-all duration-[var(--dur-fast)] bg-bg-elev-2 text-err hover:bg-err hover:text-white active:scale-95" onClick={handleCancel} title={t("composer.stop")}>
              <Square size={14} fill="currentColor" />
            </button>
          )}
          <button
            className={`inline-flex items-center justify-center w-[32px] h-[32px] border-0 rounded-full cursor-pointer shrink-0 transition-all duration-[var(--dur-fast)] active:scale-95 ${running ? "bg-bg-elev-2 text-fg-dim hover:bg-accent hover:text-accent-fg hover:scale-105" : "bg-accent text-accent-fg hover:brightness-110"} disabled:bg-bg-elev-2 disabled:text-fg-faint disabled:cursor-default disabled:hover:scale-100 disabled:active:scale-100 disabled:shadow-none`}
            style={!running && !disabled ? {boxShadow: "var(--ds-shadow-accent-btn)"} : undefined}
            onClick={submit}
            disabled={disabled || pendingPaste > 0 || (!text.trim() && attachments.length === 0 && (!running || queueLen === 0))}
            title={running ? (queueLen > 0 ? `排队发送 (${queueLen})` : t("composer.queue")) : t("composer.send")}
          >
            {running && queueLen > 0 ? (
              <span className="text-xs font-semibold leading-none">{queueLen}</span>
            ) : (
              <ArrowUp size={16} />
            )}
          </button>
        </div>

        {/* 底部工具栏 */}
        <div className="flex items-center gap-1.5 min-w-0 px-2.5 py-1.5">
          {cwd && (
            <div className="relative inline-flex min-w-0" ref={workspaceAnchorRef}>
              <button
                className={`inline-flex items-center gap-1.5 max-w-60 px-2 py-1 border-0 rounded-md bg-transparent text-fg-dim text-xs cursor-pointer transition-[color,background] duration-[var(--dur-fast)] hover:text-fg hover:bg-bg-soft disabled:cursor-default disabled:opacity-60 no-drag ${workspaceMenuOpen ? "text-fg bg-bg-soft" : ""}`}
                onClick={() => { if (!running) setWorkspaceMenuOpen((o) => !o); }}
                disabled={running}
                title={running ? t("common.busyHint") : t("status.switchFolder", { cwd })}
              >
                <FolderGit2 size={13} />
                <span className="min-w-0 truncate">{workspaceName}</span>
                <ChevronDown size={12} />
              </button>
            </div>
          )}

          {/* 统一模式按钮（V9.0：Mode + AgentMode 合并） */}
          <div className="flex gap-[3px]">
            {(["explore", "develop", "orchestrate"] as const).map((am) => {
              const labels: Record<string, string> = { explore: t("composer.modeExplore"), develop: t("composer.modeDevelop"), orchestrate: t("composer.modeOrchestrate") };
              const descs: Record<string, string> = { explore: t("composer.modeExploreDesc"), develop: t("composer.modeDevelopDesc"), orchestrate: t("composer.modeOrchestrateDesc") };
              return (
                <button key={am} type="button"
                  className={`flex items-center gap-1.5 px-2.5 py-1 border rounded-md bg-transparent text-xs cursor-pointer transition-[color,background,border,transform] duration-[var(--dur-fast)] active:scale-[0.97] ${
                    agentMode === am ? "text-accent bg-accent-soft border-accent/30 shadow-[0_0_0_1px_var(--accent-soft)]" : "text-fg-dim border-border-soft hover:text-fg hover:bg-bg-soft hover:border-fg-faint"
                  }`}
                  onClick={() => { if (agentMode !== am && onSetAgentMode) onSetAgentMode(am); }}
                  title={descs[am]}
                >
                  {labels[am]}
                </button>
              );
            })}
          </div>

          {/* YOLO 开关（V9.0：独立 toggle，仅在 develop/orchestrate 下可见） */}
          {agentMode !== "explore" && onToggleYolo && (
            <button type="button"
              className={`flex items-center gap-1.5 px-2 py-0.5 border rounded text-[10px] cursor-pointer transition-[color,background,border] duration-[var(--dur-fast)] ${
                yolo ? "text-err bg-err/10 border-err/20" : "text-fg-faint border-border-soft/50 hover:text-fg-dim hover:bg-bg-soft"
              }`}
              onClick={onToggleYolo}
              title={t("composer.yoloToggleDesc")}
            >
              {yolo ? "⚡ YOLO" : t("composer.yoloToggle")}
            </button>
          )}

{/* 快捷提示 */}
          <span className="ml-auto text-fg-faint/40 text-[10px] select-none hidden sm:inline-flex items-center gap-1.5">
            <span>/ 命令</span>
            <span>@ 文件</span>
          </span>
        </div>
      </div>
    </div>
  );
}

/** 粘贴块操作按钮 */
function ActionBtn({ title, danger, onClick, children }: { title: string; danger?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" className={`px-1.5 py-0.5 bg-transparent border-0 rounded text-fg-faint cursor-pointer text-[11px] transition-colors ${danger ? "hover:text-err hover:bg-bg-soft" : "hover:text-fg hover:bg-bg-soft"}`} title={title} onClick={onClick}>
      {children}
    </button>
  );
}
