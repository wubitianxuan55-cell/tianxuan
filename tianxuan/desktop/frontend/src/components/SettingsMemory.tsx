import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import type { MemoryFact, MemoryDoc } from "../lib/types";
import { SettingsPageShell, SettingsSection } from "./SettingsPageShell";

const TYPE_LABELS: Record<string, string> = {
  user: "用户", project: "项目", feedback: "反馈", semantic: "语义", episodic: "情景", procedural: "规程", reference: "参考",
};

export function SettingsMemory() {
  const [facts, setFacts] = useState<MemoryFact[]>([]);
  const [docs, setDocs] = useState<MemoryDoc[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");

  useEffect(() => {
    app.Memory().then((m) => { setFacts(m.facts); setDocs(m.docs); })
      .catch((e) => setErr(String(e)))
      .finally(() => setLoading(false));
  }, []);

  return (
    <SettingsPageShell title="记忆" desc="AI 通过 remember 工具保存的持久记忆，会话间共享。使用 /memory 命令查看完整面板。">
      {loading ? <p className="text-[13px] text-fg-faint">加载中…</p> :
       err ? <p className="text-[13px] text-err">{err}</p> : <>
        {docs.length > 0 && (
          <SettingsSection title={`${docs.length} 个指令文件`}>
            {docs.map((d) => (
              <div key={d.path} className="bg-bg-soft border border-border-soft rounded-lg px-3 py-2.5 mb-2">
                <div className="text-[12px] font-medium text-fg font-mono">{d.path}</div>
                <div className="text-[10px] text-fg-faint mt-0.5">{d.scope}</div>
              </div>
            ))}
          </SettingsSection>
        )}
        <SettingsSection title={`${facts.length} 条记忆`}>
          {facts.length === 0 ? (
            <p className="text-[13px] text-fg-faint">暂无保存的记忆。AI 通过 remember 工具自动写入。</p>
          ) : (
            <div className="flex flex-col gap-2">
              {facts.map((f) => (
                <div key={f.name} className="bg-bg-soft border border-border-soft rounded-lg px-3 py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="text-[12px] font-medium text-fg">{f.title || f.name}</span>
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-bg-elev-2 text-fg-faint">{TYPE_LABELS[f.type] || f.type}</span>
                  </div>
                  <div className="text-[11px] text-fg-faint mt-0.5 line-clamp-2">{f.description}</div>
                </div>
              ))}
            </div>
          )}
        </SettingsSection>
      </>}
    </SettingsPageShell>
  );
}
