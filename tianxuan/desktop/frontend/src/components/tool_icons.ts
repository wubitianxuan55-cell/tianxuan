// tool_icons.ts — 工具名→图标映射表，ToolCard.tsx 专用。
// 从 ToolCard.tsx 提取，减少主文件行数。
import {
  Activity, ArrowRightLeft, Ban, BookOpen, Brain, Bug, Check, CheckCircle,
  Clock, FilePen, FileText, FolderOpen, GitBranch, Globe, Hourglass,
  Layers, Lightbulb, List, ListTree, Plug, PlusCircle, Search, Sparkles,
  SquareTerminal, Trash2, Wrench, Zap, type LucideIcon,
} from "lucide-react";

export const ICONS: Record<string, LucideIcon> = {
  edit_file: FilePen, multi_edit: FilePen, write_file: FilePen, read_file: FileText,
  delete_range: Trash2, delete_symbol: Trash2, notebook_edit: FilePen,
  bash: SquareTerminal, bash_output: SquareTerminal, kill_shell: Ban,
  ls: FolderOpen, glob: Search, grep: Search,
  web_fetch: Globe, web_search: Globe,
  task: ListTree, run_skill: Zap, parallel_skills: Layers, install_skill: PlusCircle,
  git_status: GitBranch, git_diff: GitBranch, git_log: GitBranch, git_commit: GitBranch,
  lsp_diagnostics: Bug, lsp_definition: ArrowRightLeft, lsp_references: List,
  lsp_hover: Lightbulb, lsp_completion: Sparkles, lsp_rename: Pencil,
  memory_search: Brain, remember: Brain, read_skill: BookOpen,
  doctor: Activity, time: Clock, wait: Hourglass,
  complete_step: CheckCircle, ask: List,
};

export function mcpOr(name: string): LucideIcon {
  return name.startsWith("mcp__") ? Plug : Wrench;
}
