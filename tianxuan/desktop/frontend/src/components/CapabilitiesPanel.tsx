import { useCallback, useEffect, useMemo, useState } from "react";
import { X } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { CapabilitiesView, MCPServerInput, ServerView, SkillView } from "../lib/types";
import { ResizableDrawer } from "./ResizableDrawer";

// CapabilitiesPanel is the desktop MCP & Skills drawer — the GUI counterpart to
// the CLI's /mcp + /skill, aligning with Claude Code's Customize → Connectors:
// each server shows a connected/failed dot, transport, and tool/prompt/resource
// counts, with add / remove / retry; skills list their scope and run mode.
type CapTab = "servers" | "skills";

export function CapabilitiesPanel({
  onClose,
}: {
  onClose: () => void;
}) {
  const t = useT();
  const [view, setView] = useState<CapabilitiesView | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [confirming, setConfirming] = useState<string | null>(null);
  const [tab, setTab] = useState<CapTab>("servers");
  const [skillQuery, setSkillQuery] = useState("");
  const [expandedSkills, setExpandedSkills] = useState<Set<string>>(() => new Set());
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(() => new Set());
  const [expandedServers, setExpandedServers] = useState<Set<string>>(() => new Set());

  const reload = async () =>
    setView(await app.Capabilities().catch(() => ({ servers: [], skills: [] })));
  useEffect(() => {
    void reload();
  }, []);

  // mutate runs an MCP edit, re-reads the snapshot, and surfaces any failure as an
  // inline banner (a connect error, a missing binary, a bad URL).
  const mutate = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      await reload();
      return true;
    } catch (e) {
      setErr(String((e as Error)?.message ?? e));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const summary = useMemo(() => {
    if (!view) return "";
    return t("caps.summary", {
      connected: view.servers.filter((s) => s.status === "connected").length,
      failed: view.servers.filter((s) => s.status === "failed").length,
      skills: view.skills.length,
    });
  }, [view, t]);

  const filteredSkills = useMemo(() => {
    if (!view) return [];
    const q = skillQuery.trim().toLowerCase();
    if (!q) return view.skills;
    return view.skills.filter((sk) => {
      const text = [sk.name, `/${sk.name}`, sk.description, sk.scope, sk.runAs].join(" ").toLowerCase();
      return text.includes(q);
    });
  }, [view, skillQuery]);

  const serverGroups = useMemo(() => {
    const servers = view?.servers ?? [];
    return {
      failed: servers.filter((s) => s.status === "failed"),
      active: servers.filter((s) => s.status !== "failed"),
    };
  }, [view]);

  const toggleSkill = useCallback((name: string) => {
    setExpandedSkills((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  const toggleError = useCallback((name: string) => {
    setExpandedErrors((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  const toggleServer = useCallback((name: string) => {
    setExpandedServers((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  return (
    <ResizableDrawer onClose={onClose} subtle>
        <header className="flex items-center justify-between px-4 py-3.5 bg-bg-elev border-b border-border">
          <div>
            <div className="text-[15px] font-semibold text-fg">{t("caps.title")}</div>
            {view && <div className="mt-[3px] text-fg-faint text-[11px]">{summary}</div>}
          </div>
          <button className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border bg-bg-soft text-fg-faint rounded-[7px] cursor-pointer transition-[color,border-color,background] duration-[0.12s] hover:text-fg hover:border-fg-faint no-drag" onClick={onClose} title={t("common.close")}>
            <X size={14} />
          </button>
        </header>

        {!view ? (
          <div className="empty">{t("caps.loading")}</div>
        ) : (
          <div className="overflow-y-auto px-4 py-3.5 flex flex-col gap-[22px]">
            {err && <div className="shrink-0 px-4 py-2 text-[12.5px] bg-del-bg text-err border-b border-border-soft">{err}</div>}

            <div className="flex border-b border-border-soft mb-3" role="tablist" aria-label={t("caps.title")}>
              <button
                className={`flex-1 px-4 py-2 border-0 border-b-2 bg-transparent text-[13px] font-medium cursor-pointer transition-[color,border] duration-[0.12s] ${
                  tab === "servers" ? "text-accent border-accent" : "text-fg-dim border-transparent hover:text-fg hover:border-fg-faint"
                }`}
                role="tab" aria-selected={tab === "servers"} onClick={() => setTab("servers")}
              >{t("caps.connectorsTab")}</button>
              <button
                className={`flex-1 px-4 py-2 border-0 border-b-2 bg-transparent text-[13px] font-medium cursor-pointer transition-[color,border] duration-[0.12s] ${
                  tab === "skills" ? "text-accent border-accent" : "text-fg-dim border-transparent hover:text-fg hover:border-fg-faint"
                }`}
                role="tab" aria-selected={tab === "skills"} onClick={() => setTab("skills")}
              >{t("caps.skillsTab")}</button>
            </div>

            {tab === "servers" ? (
              <section className="mb-3">
                <div className="flex justify-end mb-2">
                  {!adding && (
                    <button className="px-2.5 py-1 text-xs" disabled={busy} onClick={() => setAdding(true)}>
                      {t("caps.addServer")}
                    </button>
                  )}
                </div>
                {serverGroups.failed.length > 0 && (
                  <FailedServersNotice
                    servers={serverGroups.failed}
                    expanded={expandedErrors}
                    onToggle={toggleError}
                    onRetry={(name) => void mutate(() => app.RetryMCPServer(name))}
                    confirming={confirming}
                    onConfirm={setConfirming}
                    onCancelConfirm={() => setConfirming(null)}
                    onRemove={(name) => mutate(() => app.RemoveMCPServer(name)).then(() => setConfirming(null))}
                    busy={busy}
                  />
                )}
                {view.servers.length === 0 && !adding && (
                  <div className="text-fg-faint text-xs text-center py-4">{t("caps.noServers")}</div>
                )}
                <ServerGroup
                  busy={busy}
                  servers={serverGroups.active}
                  expanded={expandedServers}
                  confirming={confirming}
                  onConfirm={setConfirming}
                  onCancelConfirm={() => setConfirming(null)}
                  onRemove={(name) => mutate(() => app.RemoveMCPServer(name)).then(() => setConfirming(null))}
                  onRetry={(name) => void mutate(() => app.RetryMCPServer(name))}
                  onToggle={(name, on) => void mutate(() => app.SetMCPServerEnabled(name, on))}
                  onToggleDetails={toggleServer}
                />
                {adding ? (
                  <AddServerForm busy={busy} onCancel={() => setAdding(false)} onAdd={async (input) => (await mutate(() => app.AddMCPServer(input))) && setAdding(false)} />
                ) : null}
              </section>
            ) : (
              <section className="mb-3">
                <div className="mb-2">
                  <input
                    className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
                    type="search"
                    placeholder={t("caps.searchSkills")}
                    value={skillQuery}
                    onChange={(e) => setSkillQuery(e.target.value)}
                  />
                </div>
                {view.skills.length === 0 ? (
                  <div className="py-4 text-fg-faint text-xs text-center">{t("caps.noSkills")}</div>
                ) : filteredSkills.length === 0 ? (
                  <div className="py-4 text-fg-faint text-xs text-center">{t("caps.noSkillMatches")}</div>
                ) : (
                  <div className="flex flex-col gap-2">
                    {filteredSkills.map((sk) => (
                      <SkillRow
                        key={sk.name}
                        skill={sk}
                        expanded={expandedSkills.has(sk.name)}
                        onToggle={() => toggleSkill(sk.name)}
                      />
                    ))}
                  </div>
                )}
              </section>
            )}
          </div>
        )}
    </ResizableDrawer>
  );
}

function ServerGroup({
  servers,
  expanded,
  busy,
  confirming,
  onConfirm,
  onCancelConfirm,
  onRemove,
  onRetry,
  onToggle,
  onToggleDetails,
}: {
  servers: ServerView[];
  expanded: Set<string>;
  busy: boolean;
  confirming: string | null;
  onConfirm: (name: string) => void;
  onCancelConfirm: () => void;
  onRemove: (name: string) => void;
  onRetry: (name: string) => void;
  onToggle: (name: string, on: boolean) => void;
  onToggleDetails: (name: string) => void;
}) {
  if (servers.length === 0) return null;
  return (
    <div className="cap-server-group">
      {servers.map((s) => (
        <ServerRow
          key={s.name}
          s={s}
          expanded={expanded.has(s.name)}
          busy={busy}
          confirming={confirming === s.name}
          onConfirm={() => onConfirm(s.name)}
          onCancelConfirm={onCancelConfirm}
          onRemove={() => onRemove(s.name)}
          onRetry={() => onRetry(s.name)}
          onToggle={(on) => onToggle(s.name, on)}
          onToggleDetails={() => onToggleDetails(s.name)}
        />
      ))}
    </div>
  );
}

function FailedServersNotice({
  servers,
  expanded,
  busy,
  confirming,
  onToggle,
  onRetry,
  onConfirm,
  onCancelConfirm,
  onRemove,
}: {
  servers: ServerView[];
  expanded: Set<string>;
  busy: boolean;
  confirming: string | null;
  onToggle: (name: string) => void;
  onRetry: (name: string) => void;
  onConfirm: (name: string) => void;
  onCancelConfirm: () => void;
  onRemove: (name: string) => void;
}) {
  const t = useT();
  return (
    <div className="mb-3 p-3 border border-err/20 rounded-lg bg-[color-mix(in_srgb,var(--err)_6%,transparent)]" role="status">
      <div className="flex items-center justify-between mb-2">
        <div>
          <div className="text-err text-sm font-semibold">{t("caps.failureTitle", { failed: servers.length })}</div>
          <div className="text-fg-faint text-[11px]">{t("caps.failureHint")}</div>
        </div>
      </div>
      <div className="flex flex-col gap-2">
        {servers.map((s) => {
          const open = expanded.has(s.name);
          const error = s.error || t("caps.failed");
          return (
            <div className="border border-border-soft rounded-lg overflow-hidden" key={s.name}>
              <div className="flex items-center gap-2 px-3 py-2">
                <span className="w-2 h-2 rounded-full bg-err shrink-0" />
                <div className="flex-1 min-w-0">
                  <div className="text-fg text-[13px] font-medium">{s.name}</div>
                  <div className="text-fg-faint text-[11px] truncate">{summarizeServerError(error)}</div>
                </div>
              </div>
              <div className="flex items-center gap-1 px-3 pb-2">
                {confirming === s.name ? (
                  <>
                    <button className="px-2.5 py-1 text-xs" disabled={busy} onClick={() => onRemove(s.name)}>{t("caps.confirmRemove")}</button>
                    <button className="px-2.5 py-1 text-xs" disabled={busy} onClick={onCancelConfirm}>{t("common.cancel")}</button>
                  </>
                ) : (
                  <>
                    <button className="px-2.5 py-1 text-xs" disabled={busy} onClick={() => onRetry(s.name)}>{t("caps.retry")}</button>
                    <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={() => void navigator.clipboard?.writeText(error)}>{t("common.copy")}</button>
                    <button className="px-2.5 py-1 text-xs" onClick={() => onToggle(s.name)} aria-expanded={open}>{open ? t("common.collapse") : t("caps.showLog")}</button>
                    <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-err hover:bg-bg-soft transition-colors" disabled={busy} onClick={() => onConfirm(s.name)} title={t("caps.remove")}><X size={13} /></button>
                  </>
                )}
              </div>
              {open && <pre className="m-0 p-3 bg-bg text-fg-dim text-xs leading-relaxed whitespace-pre-wrap border-t border-border-soft max-h-[200px] overflow-y-auto">{error}</pre>}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function ServerRow({
  s,
  expanded,
  busy,
  confirming,
  onConfirm,
  onCancelConfirm,
  onRemove,
  onRetry,
  onToggle,
  onToggleDetails,
}: {
  s: ServerView;
  expanded: boolean;
  busy: boolean;
  confirming: boolean;
  onConfirm: () => void;
  onCancelConfirm: () => void;
  onRemove: () => void;
  onRetry: () => void;
  onToggle: (on: boolean) => void;
  onToggleDetails: () => void;
}) {
  const t = useT();
  const actionLabel = serverActionLabel(s, t);
  const tools = s.toolList ?? [];
  const hasTools = tools.length > 0;
  const sub =
    s.status === "failed"
      ? s.error || t("caps.failed")
      : s.status === "disabled"
        ? t("caps.disabled")
        : t("caps.counts", { tools: s.tools, prompts: s.prompts, resources: s.resources });
  return (
    <div className={`border border-border-soft rounded-lg ${s.status === "disabled" ? "opacity-60" : ""}`}>
      <div className="flex items-center gap-2 px-3 py-2" title={s.error || undefined}>
        <button
          className="w-5 h-5 border-0 bg-transparent text-fg-faint cursor-pointer flex items-center justify-center text-sm disabled:opacity-30 disabled:cursor-default"
          disabled={!hasTools}
          aria-expanded={hasTools ? expanded : undefined}
          onClick={onToggleDetails}
          title={hasTools ? (expanded ? t("caps.collapseTools") : t("caps.expandTools")) : t("caps.noToolDetails")}
        >
          {hasTools ? (expanded ? "⌄" : "›") : ""}
        </button>
        <span className={`w-2 h-2 rounded-full shrink-0 ${s.status === "connected" ? "bg-ok" : s.status === "failed" ? "bg-err" : "bg-fg-faint"}`} />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-fg text-[13px] font-medium">{s.name}</span>
            <span className="text-fg-faint text-[11px] font-mono">{s.transport}</span>
          </div>
          <div className={`text-[11px] truncate ${s.status === "disabled" ? "text-fg-faint opacity-60" : "text-fg-faint"}`}>{sub}</div>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {confirming ? (
            <>
              <button className="px-2.5 py-1 text-xs" disabled={busy} onClick={onRemove}>{t("caps.confirmRemove")}</button>
              <button className="px-2.5 py-1 text-xs" disabled={busy} onClick={onCancelConfirm}>{t("common.cancel")}</button>
            </>
          ) : (
            <>
              {s.status === "failed" ? (
                <button className="px-2.5 py-1 text-xs" disabled={busy} onClick={onRetry}>{actionLabel}</button>
              ) : (
                <label className="inline-flex cursor-pointer no-drag" title={s.status === "connected" ? t("caps.disable") : t("caps.enable")}>
                  <input type="checkbox" className="peer absolute opacity-0 w-0 h-0" checked={s.status === "connected"} disabled={busy} onChange={(e) => onToggle(e.target.checked)} />
                  <span className="relative w-[30px] h-[17px] rounded-full bg-border transition-colors duration-[0.14s] peer-checked:bg-ok peer-disabled:opacity-50 peer-checked:[&>span]:translate-x-[13px]">
                    <span className="absolute top-0.5 left-0.5 w-[13px] h-[13px] rounded-full bg-bg-elev transition-transform duration-[0.14s]" />
                  </span>
                </label>
              )}
              <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-err hover:bg-bg-soft transition-colors" disabled={busy} onClick={onConfirm} title={t("caps.remove")}><X size={13} /></button>
            </>
          )}
        </div>
      </div>
      {hasTools && expanded && (
        <div className="border-t border-border-soft px-3 py-2">
          <div className="text-fg-faint text-[11px] font-medium mb-1">{t("caps.tools")}</div>
          {tools.map((tool) => (
            <div className="flex items-center gap-2 px-2 py-1" key={tool.name}>
              <span className="font-mono text-fg text-[13px]">{tool.name}</span>
              {tool.description && <span className="text-fg-faint text-[11px] truncate">{tool.description}</span>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function summarizeServerError(error: string): string {
  const normalized = error.replace(/\s+/g, " ").trim();
  const plugin = normalized.match(/plugin "([^"]+)"/i)?.[1];
  const npmCode = normalized.match(/\bnpm error code ([A-Z0-9_]+)/i)?.[1];
  const errno = normalized.match(/\berrno (-?\d+)/i)?.[1];
  const reason = npmCode
    ? `npm ${npmCode}${errno ? ` (${errno})` : ""}`
    : normalized.split(/(?:\.\s+|\n)/)[0];
  const summary = plugin ? `${plugin}: ${reason}` : reason;
  return summary.length > 180 ? `${summary.slice(0, 176).trim()}…` : summary;
}

function serverActionLabel(s: ServerView, t: ReturnType<typeof useT>): string {
  const err = (s.error || "").toLowerCase();
  if (err.includes("401") || err.includes("unauthorized")) return t("caps.reauthorize");
  if (
    err.includes("command not found") ||
    err.includes("executable file not found") ||
    err.includes("no such file") ||
    err.includes("enoent")
  ) {
    return t("caps.checkCommand");
  }
  return t("caps.retry");
}

function SkillRow({
  skill,
  expanded,
  onToggle,
}: {
  skill: SkillView;
  expanded: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const summary = summarizeSkillDescription(skill.description);
  const canExpand = summary !== skill.description;
  return (
    <button
      className={`w-full text-left border border-border-soft rounded-lg p-3 bg-transparent cursor-pointer transition-[border-color,background] duration-[0.12s] hover:border-accent/30 hover:bg-bg-soft active:bg-bg-elev ${
        expanded ? "border-accent/30 bg-bg-elev" : ""
      }`}
      type="button"
      onClick={onToggle}
      aria-expanded={expanded}
      title={skill.description}
    >
      <div className="flex items-center gap-2.5 mb-1">
        <span className="w-8 h-8 flex items-center justify-center rounded-md bg-accent-soft text-accent font-mono text-base font-bold shrink-0">/</span>
        <span className="flex-1 min-w-0 flex flex-col gap-0.5">
          <span className="text-fg text-[13px] font-semibold font-mono">{skill.name}</span>
          <span className="flex items-center gap-1">
            <span className={`text-[10px] px-1.5 py-px rounded font-medium ${
              skill.scope === "project" ? "bg-ok/15 text-ok" : "bg-fg-faint/15 text-fg-faint"
            }`}>{skillScopeLabel(skill.scope, t)}</span>
            {skill.runAs === "subagent" && <span className="text-[10px] px-1.5 py-px rounded font-medium bg-accent/15 text-accent">{t("caps.subagent")}</span>}
          </span>
        </span>
      </div>
      <div className={`text-fg-dim text-[12px] leading-snug ${expanded ? "" : "line-clamp-2"}`}>
        {expanded ? skill.description : summary}
      </div>
      {canExpand && <div className="mt-1 text-fg-faint text-[11px]">{expanded ? t("common.collapse") : t("common.expand")}</div>}
    </button>
  );
}

function skillScopeLabel(scope: string, t: ReturnType<typeof useT>): string {
  switch (scope) {
    case "builtin":
      return t("caps.skillScopeBuiltin");
    case "project":
      return t("caps.skillScopeProject");
    case "custom":
      return t("caps.skillScopeCustom");
    case "global":
      return t("caps.skillScopeGlobal");
    default:
      return scope;
  }
}

function summarizeSkillDescription(description: string): string {
  const normalized = description.replace(/\s+/g, " ").trim();
  if (normalized.length <= 132) return normalized;
  const sentence = normalized.match(/^.{48,132}?[。.!?；;，,]/u)?.[0]?.trim();
  if (sentence && sentence.length >= 48) return sentence.replace(/[。.!?；;，,]$/u, "");
  return `${normalized.slice(0, 128).trim()}…`;
}

function AddServerForm({
  busy,
  onCancel,
  onAdd,
}: {
  busy: boolean;
  onCancel: () => void;
  onAdd: (input: MCPServerInput) => void;
}) {
  const t = useT();
  const [name, setName] = useState("");
  const [transport, setTransport] = useState("stdio");
  const [command, setCommand] = useState("");
  const [url, setUrl] = useState("");
  const [env, setEnv] = useState("");

  const isStdio = transport === "stdio";
  const ready = name.trim() !== "" && (isStdio ? command.trim() !== "" : url.trim() !== "");

  const submit = () => {
    const parts = command.trim().split(/\s+/).filter(Boolean);
    const envMap: Record<string, string> = {};
    for (const line of env.split("\n")) {
      const eq = line.indexOf("=");
      if (eq > 0) envMap[line.slice(0, eq).trim()] = line.slice(eq + 1).trim();
    }
    onAdd({
      name: name.trim(),
      transport,
      command: isStdio ? (parts[0] ?? "") : "",
      args: isStdio ? parts.slice(1) : [],
      url: isStdio ? "" : url.trim(),
      env: envMap,
    });
  };

  return (
    <div className="flex flex-col gap-2 p-3 border border-border-soft rounded-lg mb-2">
      <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("caps.namePlaceholder")} value={name} onChange={(e) => setName(e.target.value)} />
      <label className="text-fg-dim text-[13px] shrink-0">{t("caps.transport")}</label>
      <select className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={transport} onChange={(e) => setTransport(e.target.value)}>
        <option value="stdio">stdio</option>
        <option value="http">http</option>
        <option value="sse">sse</option>
      </select>
      {isStdio ? (
        <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("caps.commandPlaceholder")} value={command} onChange={(e) => setCommand(e.target.value)} />
      ) : (
        <input className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent" placeholder={t("caps.urlPlaceholder")} value={url} onChange={(e) => setUrl(e.target.value)} />
      )}
      <label className="text-fg-dim text-[13px] shrink-0">{t("caps.envLabel")}</label>
      <textarea className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] p-2 outline-none resize-y min-h-[60px] focus:border-accent" value={env} onChange={(e) => setEnv(e.target.value)} placeholder={t("caps.envPlaceholder")} spellCheck={false} />
      <div className="flex justify-end gap-2 mt-2">
        <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={onCancel} disabled={busy}>
          {t("common.cancel")}
        </button>
        <button className="btn--primary" onClick={submit} disabled={busy || !ready}>
          {t("caps.add")}
        </button>
      </div>
    </div>
  );
}
