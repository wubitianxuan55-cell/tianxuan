import { useMemo, useRef, useState } from "react";
import { Cpu, ChevronDown, Search } from "lucide-react";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";

type Counts = Record<string, number>;

/** Compact descriptions mirroring Go's compactDesc map + skill tools */
const TOOL_DESC: Record<string, string> = {
  // 文件
  read_file: "读取文件内容(可选行范围/分页)",
  write_file: "写入/覆盖文件(自动建父目录)",
  edit_file: "精确替换文件字符串(须全局唯一)",
  multi_edit: "原子化批量编辑(单文件N步依次执行)",
  delete_range: "删除文件连续行(起止锚点定位)",
  delete_symbol: "删除Go符号(函数/类型/接口等,AST解析)",
  glob: "通配符匹配文件名(支持**递归)",
  grep: "正则搜索文件内容(返回path:行:文本,限200条)",
  ls: "列目录条目(子目录带/)",
  notebook_edit: "编辑Jupyter Notebook单元格(.ipynb)",
  // 命令
  bash: "执行shell命令(合并stdout+stderr,限2分钟)",
  bash_output: "读取后台任务的增量输出(不阻塞)",
  wait: "阻塞等待后台任务结束(可设超时)",
  kill_shell: "终止后台任务(SIGTERM→SIGKILL)",
  // 版本
  git_status: "显示工作区状态(分支/暂存/未暂存/未跟踪/冲突)",
  git_diff: "显示行级别变更(--staged可选,path可限文件)",
  git_log: "显示提交历史(支持count/path/author过滤)",
  git_commit: "提交暂存变更(可stage_all/amend/自动生成消息)",
  // 网络
  web_fetch: "抓取URL纯文本(去标签,SSRF安全)",
  web_search: "搜索公开网页(通过DuckDuckGo)",
  // 任务/规划
  todo_write: "更新任务清单(全量替换,最多一个进行中)",
  complete_step: "完成计划步骤(附验证证据,空证据拒绝)",
  ask: "向用户提供多选项问题",
  // 系统
  doctor: "运行系统诊断(Go环境/项目结构/OS)",
  time: "获取当前日期时间(可指定时区)",
  // 子代理
  task: "派发子代理执行聚焦子任务",
  explore: "隔离子代理——只读代码库调查",
  research: "隔离子代理——web搜索+代码阅读",
  review: "隔离子代理——审查分支diff",
  security_review: "隔离子代理——安全审查分支diff",
  // 技能
  run_skill: "调用Skills索引中的playbook",
  parallel_skills: "并行派发多个子代理技能",
  install_skill: "编写并保存新技能",
  // 记忆
  remember: "保存持久事实到项目记忆",
  forget: "通过名称删除已保存记忆",
  memory_search: "按关键词搜索已保存记忆",
};

interface Section {
  title: string;
  items: string[];
}

const SECTIONS: Section[] = [
  {
    title: "文件",
    items: ["read_file", "write_file", "edit_file", "multi_edit", "delete_range", "delete_symbol", "glob", "grep", "ls", "notebook_edit"],
  },
  {
    title: "命令",
    items: ["bash", "bash_output", "wait", "kill_shell"],
  },
  {
    title: "版本",
    items: ["git_status", "git_diff", "git_log", "git_commit"],
  },
  {
    title: "网络",
    items: ["web_fetch", "web_search"],
  },
  {
    title: "任务",
    items: ["todo_write", "complete_step", "ask"],
  },
  {
    title: "子代理",
    items: ["task", "explore", "research", "review", "security_review"],
  },
  {
    title: "技能",
    items: ["run_skill", "parallel_skills", "install_skill"],
  },
  {
    title: "记忆",
    items: ["remember", "forget", "memory_search"],
  },
  {
    title: "系统",
    items: ["doctor", "time"],
  },
];

function ToolCard({ name, count }: { name: string; count: number }) {
  const active = count > 0;
  const desc = TOOL_DESC[name];
  return (
    <div
      className={`flex items-start gap-1.5 px-2 py-1.5 rounded-md border border-border-soft bg-bg cursor-default ${
        active ? "border-accent-soft bg-sidebar-active" : ""
      }`}
      title={desc ?? name}
    >
      <span className={`w-1.5 h-1.5 mt-[5px] rounded-full shrink-0 ${active ? "bg-accent" : "bg-border-soft"}`} />
      <span className="flex-1 min-w-0 flex flex-col gap-0.5 leading-[1.25]">
        <span className={`font-mono text-[10.5px] truncate ${active ? "text-accent font-semibold" : "text-fg-dim"}`}>
          {name}
        </span>
        {desc && <span className="text-[10px] text-fg-faint leading-[1.3] line-clamp-1">{desc}</span>}
      </span>
      <span className={`shrink-0 font-mono text-[11px] font-semibold mt-px ${active ? "text-accent" : "text-fg-faint"}`}>
        {count}
      </span>
    </div>
  );
}

function ToolGroup({
  title,
  items,
  counts,
  defaultOpen,
}: {
  title: string;
  items: string[];
  counts: Counts;
  defaultOpen: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const ref = useRef<HTMLDivElement>(null);
  useGSAPCollapse(ref, open, { duration: 0.18 });

  const activeCount = items.filter((n) => (counts[n] ?? 0) > 0).length;

  return (
    <div className="px-1.5 py-0.5">
      <button
        className="flex items-center gap-1 w-full px-1 py-1.5 bg-transparent border-0 text-left cursor-pointer hover:bg-bg-soft rounded transition-colors"
        onClick={() => setOpen((v) => !v)}
      >
        <ChevronDown
          size={10}
          className={`text-fg-faint transition-transform duration-150 ${open ? "rotate-0" : "-rotate-90"}`}
        />
        <span className="text-[10px] font-semibold uppercase tracking-[0.5px] text-fg-faint">{title}</span>
        {activeCount > 0 && (
          <span className="ml-auto text-[9px] font-mono text-accent">{activeCount}</span>
        )}
      </button>
      <div ref={ref} style={{ overflow: "hidden" }}>
        <div className="flex flex-col gap-0.5 pt-0.5 pb-1">
          {items.map((name) => (
            <ToolCard key={name} name={name} count={counts[name] ?? 0} />
          ))}
        </div>
      </div>
    </div>
  );
}

/** RuntimePanel — 右边栏"工具"标签页，按类别分组+搜索+紧凑描述 */
export function RuntimePanel({ counts }: { counts: Counts }) {
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const totalTools = SECTIONS.reduce((sum, s) => sum + s.items.length, 0);
  const activeTotal = useMemo(
    () => SECTIONS.reduce((sum, s) => sum + s.items.filter((n) => (counts[n] ?? 0) > 0).length, 0),
    [counts],
  );

  const filteredSections = useMemo(() => {
    if (!query.trim()) return SECTIONS;
    const q = query.toLowerCase();
    return SECTIONS
      .map((sec) => ({
        ...sec,
        items: sec.items.filter(
          (name) =>
            name.toLowerCase().includes(q) ||
            (TOOL_DESC[name] ?? "").toLowerCase().includes(q),
        ),
      }))
      .filter((sec) => sec.items.length > 0);
  }, [query]);

  const hasResults = filteredSections.length > 0;

  return (
    <div className="flex flex-col overflow-hidden text-xs h-full">
      {/* Header */}
      <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border-soft text-fg-dim font-semibold text-[11px] shrink-0">
        <Cpu size={12} />
        <span>工具</span>
        <span className="ml-auto text-[10px] font-mono text-fg-faint/50">
          {activeTotal > 0 ? `${activeTotal}/${totalTools}` : totalTools}
        </span>
      </div>

      {/* Search */}
      <div className="flex items-center gap-1.5 mx-2 my-1.5 px-2 h-7 border border-border rounded-md bg-bg text-fg-faint shrink-0">
        <Search size={12} />
        <input
          ref={inputRef}
          className="flex-1 min-w-0 border-0 outline-none bg-transparent text-fg text-[11.5px] placeholder:text-fg-faint"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="搜索工具…"
        />
        {query && (
          <button
            className="border-0 bg-transparent text-fg-faint cursor-pointer p-0 leading-none hover:text-fg"
            onClick={() => { setQuery(""); inputRef.current?.focus(); }}
          >
            ✕
          </button>
        )}
      </div>

      {/* Tool list */}
      <div className="flex-1 min-h-0 overflow-y-auto pb-2">
        {!hasResults ? (
          <div className="empty-state">无匹配工具</div>
        ) : (
          filteredSections.map((sec) => (
            <ToolGroup
              key={sec.title}
              title={sec.title}
              items={sec.items}
              counts={counts}
              defaultOpen={sec.title === "文件" || filteredSections.length <= 3}
            />
          ))
        )}
      </div>
    </div>
  );
}
