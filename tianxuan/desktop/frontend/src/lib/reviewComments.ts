// reviewComments.ts — diff 行内评论的纯函数存储（Codex review 注释蒸馏）。
// 按 workspace（cwd）与文件路径分组，localStorage 持久化，跨会话保留。
export interface ReviewComment {
  id: string;
  path: string;
  line: number;
  text: string;
  at: number;
}

export type CommentStorage = Pick<Storage, "getItem" | "setItem">;

function keyFor(cwd: string): string {
  return `tianxuan.reviewComments.${encodeURIComponent(cwd || "default")}`;
}

function isComment(v: unknown): v is ReviewComment {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;
  return (
    typeof o.id === "string" &&
    typeof o.path === "string" &&
    typeof o.line === "number" &&
    typeof o.text === "string" &&
    typeof o.at === "number"
  );
}

function loadMap(storage: CommentStorage, cwd: string): Record<string, ReviewComment[]> {
  try {
    const raw = storage.getItem(keyFor(cwd));
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) return {};
    const out: Record<string, ReviewComment[]> = {};
    for (const [path, list] of Object.entries(parsed as Record<string, unknown>)) {
      if (Array.isArray(list)) {
        const valid = list.filter(isComment);
        if (valid.length > 0) out[path] = valid;
      }
    }
    return out;
  } catch {
    return {};
  }
}

function saveMap(storage: CommentStorage, cwd: string, map: Record<string, ReviewComment[]>): void {
  storage.setItem(keyFor(cwd), JSON.stringify(map));
}

export function loadAllComments(storage: CommentStorage, cwd: string): ReviewComment[] {
  return Object.values(loadMap(storage, cwd)).flat();
}

export function loadPathComments(storage: CommentStorage, cwd: string, path: string): ReviewComment[] {
  return loadMap(storage, cwd)[path] ?? [];
}

function newCommentId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `c-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

export function addComment(
  storage: CommentStorage,
  cwd: string,
  path: string,
  line: number,
  text: string,
): ReviewComment[] {
  const trimmed = text.trim();
  if (trimmed === "" || line <= 0) return loadPathComments(storage, cwd, path);
  const map = loadMap(storage, cwd);
  const list = map[path] ?? [];
  const next: ReviewComment[] = [
    ...list,
    { id: newCommentId(), path, line, text: trimmed, at: Date.now() },
  ];
  map[path] = next;
  saveMap(storage, cwd, map);
  return next;
}

export function removeComment(storage: CommentStorage, cwd: string, path: string, id: string): ReviewComment[] {
  const map = loadMap(storage, cwd);
  const list = map[path] ?? [];
  const next = list.filter((c) => c.id !== id);
  if (next.length === 0) {
    delete map[path];
  } else {
    map[path] = next;
  }
  saveMap(storage, cwd, map);
  return next;
}

// formatCommentsForAI 把评论拼成 "path:line: text" 文本，发给 agent 逐条处理。
export function formatCommentsForAI(comments: ReviewComment[]): string {
  return comments.map((c) => `${c.path}:${c.line}: ${c.text}`).join("\n");
}
