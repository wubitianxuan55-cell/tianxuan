// Tool catalog presentation helpers for the right-drawer "工具" tab.
// Pure functions (no React, no backend calls) so the panel stays a thin
// renderer and the grouping/filtering/sorting logic is unit-testable.

export type ToolCategory =
  | "file"
  | "command"
  | "git"
  | "network"
  | "plan"
  | "subagent"
  | "skill"
  | "memory"
  | "code"
  | "other";

export interface CatalogTool {
  name: string;
  description: string; // display description (compact, single line)
  fullDescription: string; // full backend description
  readOnly: boolean;
}

export const CATEGORY_ORDER: ToolCategory[] = [
  "file",
  "command",
  "git",
  "network",
  "plan",
  "subagent",
  "skill",
  "memory",
  "code",
  "other",
];

export const CATEGORY_LABELS: Record<ToolCategory, string> = {
  file: "文件",
  command: "命令",
  git: "版本",
  network: "网络",
  plan: "任务/规划",
  subagent: "子代理",
  skill: "技能",
  memory: "记忆",
  code: "代码检索",
  other: "其他",
};

// Category rules in priority order. exact matches the whole name, prefix matches
// a leading namespace (e.g. git_*), contains matches a substring (e.g. *skill*).
interface CategoryRule {
  exact?: string[];
  prefix?: string[];
  contains?: string[];
}

const CATEGORY_RULES: Record<Exclude<ToolCategory, "other">, CategoryRule> = {
  file: {
    exact: [
      "read_file",
      "write_file",
      "edit_file",
      "multi_edit",
      "edit_lines",
      "delete_range",
      "delete_symbol",
      "move_file",
      "glob",
      "grep",
      "ls",
      "notebook_edit",
    ],
  },
  command: { exact: ["wait", "kill_shell"], contains: ["bash"] },
  git: { prefix: ["git_"] },
  network: { prefix: ["web_"] },
  plan: { exact: ["todo_write", "complete_step", "ask", "verify_gate"] },
  subagent: { exact: ["task", "explore", "research", "review", "security_review"] },
  skill: { contains: ["skill"] },
  memory: { exact: ["remember", "forget"], prefix: ["memory_"] },
  code: { exact: ["code_index", "search_large_output"], prefix: ["lsp_", "codegraph_"] },
};

export function categorizeTool(name: string): ToolCategory {
  for (const category of CATEGORY_ORDER) {
    if (category === "other") continue;
    const rule = CATEGORY_RULES[category];
    if (rule.exact?.includes(name)) return category;
    if (rule.prefix?.some((p) => name.startsWith(p))) return category;
    if (rule.contains?.some((c) => name.includes(c))) return category;
  }
  return "other";
}

export interface CatalogFilter {
  tools: CatalogTool[];
  query: string;
  onlyUsed: boolean;
  counts: Record<string, number>;
}

export function filterCatalog(
  tools: CatalogTool[],
  query: string,
  onlyUsed: boolean,
  counts: Record<string, number>,
): CatalogTool[] {
  const q = query.trim().toLowerCase();
  return tools.filter((t) => {
    if (onlyUsed && (counts[t.name] ?? 0) <= 0) return false;
    if (!q) return true;
    return (
      t.name.toLowerCase().includes(q) ||
      t.description.toLowerCase().includes(q) ||
      t.fullDescription.toLowerCase().includes(q)
    );
  });
}

export function sortCatalog(tools: CatalogTool[], counts: Record<string, number>): CatalogTool[] {
  return [...tools].sort((a, b) => {
    const usedA = (counts[a.name] ?? 0) > 0 ? 1 : 0;
    const usedB = (counts[b.name] ?? 0) > 0 ? 1 : 0;
    return usedB - usedA;
  });
}

export interface CatalogGroup {
  category: ToolCategory;
  label: string;
  tools: CatalogTool[];
}

export function groupByCategory(tools: CatalogTool[]): CatalogGroup[] {
  const byCategory = new Map<ToolCategory, CatalogTool[]>();
  for (const t of tools) {
    const category = categorizeTool(t.name);
    const arr = byCategory.get(category) ?? [];
    arr.push(t);
    byCategory.set(category, arr);
  }
  const out: CatalogGroup[] = [];
  for (const category of CATEGORY_ORDER) {
    const items = byCategory.get(category);
    if (items && items.length > 0) {
      out.push({ category, label: CATEGORY_LABELS[category], tools: items });
    }
  }
  return out;
}

export interface HighlightPart {
  text: string;
  hit: boolean;
}

export function highlightParts(text: string, query: string): HighlightPart[] {
  const q = query.trim();
  if (!q) return [{ text, hit: false }];
  const lower = text.toLowerCase();
  const needle = q.toLowerCase();
  const parts: HighlightPart[] = [];
  let cursor = 0;
  for (;;) {
    const idx = lower.indexOf(needle, cursor);
    if (idx < 0) {
      if (cursor < text.length) parts.push({ text: text.slice(cursor), hit: false });
      break;
    }
    if (idx > cursor) parts.push({ text: text.slice(cursor, idx), hit: false });
    parts.push({ text: text.slice(idx, idx + q.length), hit: true });
    cursor = idx + q.length;
  }
  return parts.length > 0 ? parts : [{ text, hit: false }];
}
