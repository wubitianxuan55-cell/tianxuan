// WorktreesPanel — git worktree 桌面管理（Codex Local environments 的
// worktree 隔离蒸馏）：列出当前仓库全部 worktree，支持创建（add -b）、
// 切换（SwitchWorkspace 激活）、删除（--force，可连带删分支）。
import { useCallback, useEffect, useState } from "react";
import {
  GitBranch,
  GitFork,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { app } from "../lib/bridge";
import type { WorktreeView } from "../lib/types";
import { useT } from "../lib/i18n";

export function WorktreesPanel({ onActivate }: { onActivate?: (path: string) => void }) {
  const t = useT();
  const [items, setItems] = useState<WorktreeView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [path, setPath] = useState("");
  const [branch, setBranch] = useState("");
  const [base, setBase] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setItems(await app.Worktrees());
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  const create = async () => {
    if (!path.trim() || !branch.trim() || busy) return;
    setBusy(true);
    setMessage("");
    try {
      await app.AddWorktree(path.trim(), branch.trim(), base.trim());
      setPath("");
      setBranch("");
      setBase("");
      setMessage(t("worktrees.created") ?? "已创建");
      await refresh();
    } catch (e) {
      setMessage(String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (it: WorktreeView) => {
    if (it.current || busy) return;
    const branchSuffix = it.branch ? ` (${it.branch})` : "";
    if (!window.confirm(`${t("worktrees.removeConfirm") ?? "删除 worktree"} ${it.path}${branchSuffix}？`)) return;
    setBusy(true);
    setMessage("");
    try {
      await app.RemoveWorktree(it.path, it.branch);
      await refresh();
    } catch (e) {
      setMessage(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border-soft shrink-0">
        <GitBranch size={13} className="text-accent" />
        <span className="text-xs font-medium text-fg">{t("worktrees.title") ?? "工作树"}</span>
        <button
          className="ml-auto inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:bg-bg-soft hover:text-fg"
          title={t("worktrees.refresh") ?? "刷新"}
          onClick={() => void refresh()}
        >
          <RefreshCw size={13} />
        </button>
      </div>

      {/* 新建表单 */}
      <div className="shrink-0 px-3 py-2.5 border-b border-border-soft space-y-1.5">
        <input
          className="w-full border border-border-soft rounded-md bg-bg-soft px-2 py-1.5 text-xs text-fg outline-none placeholder:text-fg-faint focus:border-accent/40 font-mono"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          placeholder={t("worktrees.newPath") ?? "新 worktree 路径（绝对或相对）"}
          spellCheck={false}
        />
        <div className="flex items-center gap-1.5">
          <input
            className="flex-1 min-w-0 border border-border-soft rounded-md bg-bg-soft px-2 py-1.5 text-xs text-fg outline-none placeholder:text-fg-faint focus:border-accent/40 font-mono"
            value={branch}
            onChange={(e) => setBranch(e.target.value)}
            placeholder={t("worktrees.newBranch") ?? "新分支名"}
            spellCheck={false}
          />
          <input
            className="flex-1 min-w-0 border border-border-soft rounded-md bg-bg-soft px-2 py-1.5 text-xs text-fg outline-none placeholder:text-fg-faint focus:border-accent/40 font-mono"
            value={base}
            onChange={(e) => setBase(e.target.value)}
            placeholder={t("worktrees.base") ?? "基线（默认 HEAD）"}
            spellCheck={false}
          />
          <button
            className="inline-flex items-center justify-center w-8 h-8 border-0 rounded-md bg-accent text-accent-fg cursor-pointer transition-all duration-150 hover:brightness-110 active:scale-95 disabled:opacity-40 disabled:cursor-default shrink-0"
            disabled={busy || !path.trim() || !branch.trim()}
            title={t("worktrees.create") ?? "创建"}
            onClick={() => void create()}
          >
            {busy ? <Loader2 size={13} className="animate-spin" /> : <Plus size={13} />}
          </button>
        </div>
      </div>

      {message && <div className="shrink-0 px-3 py-1.5 text-[11px] text-accent font-mono break-all">{message}</div>}
      {error && <div className="shrink-0 px-3 py-1.5 text-[11px] text-err font-mono break-all">{error}</div>}

      <div className="flex-1 min-h-0 overflow-y-auto px-2 py-2 space-y-1">
        {loading ? (
          <div className="flex flex-col items-center gap-2 py-10 text-fg-faint/60">
            <Loader2 size={18} className="animate-spin text-accent" />
            <span className="text-xs">{t("worktrees.loading") ?? "加载中…"}</span>
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-10 text-fg-faint/50">
            <GitFork size={22} />
            <span className="text-xs">{t("worktrees.empty") ?? "当前仓库没有 worktree"}</span>
          </div>
        ) : (
          items.map((it) => (
            <div
              key={it.path}
              className={`flex items-center gap-2 px-2.5 py-2 rounded-md border ${it.current ? "border-accent/30 bg-accent/8" : "border-border-soft bg-bg-soft/40"} transition-colors`}
            >
              <span className={`inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-bold shrink-0 ${it.current ? "bg-accent/20 text-accent" : "bg-bg-soft text-fg-faint"}`}>
                {it.branch ? it.branch.charAt(0).toUpperCase() : "D"}
              </span>
              <span className="flex-1 min-w-0">
                <span className="flex items-center gap-1.5 min-w-0">
                  <span className="text-xs text-fg truncate">{it.branch || (t("worktrees.detached") ?? "detached")}</span>
                  {it.current && <span className="shrink-0 text-[9px] px-1 py-px rounded-full bg-accent/20 text-accent">{t("worktrees.current") ?? "当前"}</span>}
                </span>
                <span className="block text-[10.5px] text-fg-faint font-mono truncate">{it.path}</span>
              </span>
              {!it.current && onActivate && (
                <button
                  className="inline-flex items-center justify-center w-6 h-6 border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-accent hover:bg-bg-soft shrink-0"
                  title={t("worktrees.activate") ?? "切换到此工作树"}
                  onClick={() => { setMessage(""); onActivate(it.path); }}
                >
                  <Play size={12} />
                </button>
              )}
              {!it.current && (
                <button
                  className="inline-flex items-center justify-center w-6 h-6 border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-err hover:bg-bg-soft shrink-0"
                  title={t("worktrees.remove") ?? "删除"}
                  disabled={busy}
                  onClick={() => void remove(it)}
                >
                  <Trash2 size={12} />
                </button>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
