import { Cpu } from "lucide-react";

type Counts = Record<string, number>;

const SECTIONS = [
  {
    title: "工具",
    items: ["bash", "read_file", "write_file", "edit_file", "multi_edit", "grep", "glob", "ls", "web_fetch", "web_search"],
  },
  {
    title: "子代理",
    items: ["task", "explore", "research", "review", "security_review"],
  },
];

function ToolCardView({ name, count }: { name: string; count: number }) {
  const active = count > 0;
  return (
    <div className={`flex items-center gap-[5px] px-[7px] py-[5px] rounded-md border border-border-soft bg-bg ${active ? "border-accent-soft bg-sidebar-active" : ""}`}>
      <span className={`w-[5px] h-[5px] rounded-full shrink-0 ${active ? "bg-accent" : "bg-border-soft"}`} />
      <span className={`font-mono text-[10.5px] flex-1 overflow-hidden text-ellipsis whitespace-nowrap ${active ? "text-accent font-semibold" : "text-fg-dim"}`}>{name}</span>
      <span className={`font-mono text-[11px] font-semibold ${active ? "text-accent" : "text-fg-faint"}`}>{count}</span>
    </div>
  );
}

export function RuntimePanel({ counts }: { counts: Counts }) {
  return (
    <div className="flex flex-col overflow-y-auto text-xs">
      <div className="flex items-center gap-1.5 px-2.5 py-[9px] border-b border-border-soft text-fg-dim font-semibold text-[11px]">
        <Cpu size={12} />
        <span>工具</span>
      </div>
      <div className="py-1">
        {SECTIONS.map((sec) => (
          <div className="px-1.5 py-1" key={sec.title}>
            <div className="text-[10px] font-semibold uppercase tracking-[0.5px] text-fg-faint py-1.5 px-1">{sec.title}</div>
            <div className="flex flex-col gap-[3px]">
              {sec.items.map((name) => (
                <ToolCardView key={name} name={name} count={counts[name] || 0} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
