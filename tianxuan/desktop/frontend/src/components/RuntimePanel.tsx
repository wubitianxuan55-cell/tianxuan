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
    <div className={`rt-card ${active ? "rt-card--active" : ""}`}>
      <span className="rt-card__dot" />
      <span className="rt-card__name">{name}</span>
      <span className="rt-card__count">{count}</span>
    </div>
  );
}

export function RuntimePanel({ counts }: { counts: Counts }) {
  return (
    <div className="rt-panel">
      <div className="rt-panel__head">
        <Cpu size={12} />
        <span>工具</span>
      </div>
      <div className="rt-panel__list">
        {SECTIONS.map((sec) => (
          <div className="rt-section" key={sec.title}>
            <div className="rt-section__title">{sec.title}</div>
            <div className="rt-section__grid">
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
