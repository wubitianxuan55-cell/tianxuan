import { useEffect, useState } from "react";
import { Blocks } from "lucide-react";
import { app } from "../lib/bridge";
import type { CapabilitiesView } from "../lib/types";

/** SkillsPanel — 右边栏"技能"标签页，显示全部已发现技能及使用次数 */
export function SkillsPanel({ counts }: { counts: Record<string, number> }) {
  const [skills, setSkills] = useState<CapabilitiesView["skills"]>([]);
  useEffect(() => {
    app.Capabilities().then((v) => setSkills(v.skills)).catch(() => setSkills([]));
  }, []);

  return (
    <div className="rt-panel">
      <div className="rt-panel__head">
        <Blocks size={12} />
        <span>技能</span>
      </div>
      <div className="rt-panel__list">
        {skills.length === 0 ? (
          <div className="rt-panel__empty">加载中...</div>
        ) : (
          <div className="rt-section">
            <div className="rt-section__grid">
              {skills.map((sk) => (
                <div
                  key={sk.name}
                  className={`rt-card ${(counts[sk.name] ?? 0) > 0 ? "rt-card--active" : ""}`}
                  title={sk.description}
                >
                  <span className="rt-card__dot" />
                  <span className="rt-card__name">{sk.name}</span>
                  <span className="rt-card__count">{counts[sk.name] ?? 0}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
