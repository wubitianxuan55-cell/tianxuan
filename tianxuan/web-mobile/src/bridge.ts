// mobile bridge — 复用 web/src/bridge.ts 的 Wails polyfill 模式。
// 将 window.go.main.App 和 window.runtime 替换为 HTTP/SSE 实现，
// 桌面端的 zustand store 和所有组件原封不动运行。

import type {
  WireEvent, HistoryMessage, ContextInfo, Meta, CapabilitiesView,
  SessionMeta, SettingsView, MemoryView, CheckpointMeta, SlashArgsResult,
  DirEntry, FilePreview, WorkspaceView, BalanceInfo, JobView,
  CommandInfo, QuestionAnswer, MCPServerInput, ModelInfo, ProviderView, UpdateInfo,
} from "@shared/lib/types";

// ── Token ────────────────────────────────────────────────────────────
const TOKEN_KEY = "tianxuan_mobile_token";
function getToken(): string {
  const p = new URLSearchParams(location.search);
  const t = p.get("token");
  if (t) { localStorage.setItem(TOKEN_KEY, t); history.replaceState({}, "", location.pathname + location.hash); return t; }
  return localStorage.getItem(TOKEN_KEY) || "";
}
function authHeaders(): Record<string, string> {
  const h: Record<string, string> = { "Content-Type": "application/json" };
  const t = getToken(); if (t) h["Authorization"] = `Bearer ${t}`;
  return h;
}

// ── SSE ──────────────────────────────────────────────────────────────
const listeners = new Set<(e: WireEvent) => void>();
let es: EventSource | null = null;
let rt: ReturnType<typeof setTimeout> | null = null;

function doConnect() {
  if (es) { es.close(); es = null; } if (rt) { clearTimeout(rt); rt = null; }
  const t = getToken();
  es = new EventSource(t ? `/events?token=${encodeURIComponent(t)}` : "/events");
  es.onmessage = (m) => { try { const e = JSON.parse(m.data) as WireEvent; listeners.forEach(l => l(e)); } catch {} };
  es.onerror = () => { if (es?.readyState === EventSource.CLOSED) { es.close(); es = null; rt = setTimeout(doConnect, 2000); } };
}

export function onEvent(cb: (e: WireEvent) => void): () => void {
  listeners.add(cb); if (!es) doConnect();
  return () => { listeners.delete(cb); };
}

// ── HTTP helpers ─────────────────────────────────────────────────────
async function post(path: string, body?: unknown): Promise<void> {
  const r = await fetch(path, { method: "POST", headers: authHeaders(), body: body !== undefined ? JSON.stringify(body) : undefined });
  if (!r.ok) throw new Error(`${r.status}`);
}
async function get<T>(path: string): Promise<T> {
  const r = await fetch(path, { headers: authHeaders() });
  if (!r.ok) throw new Error(`${r.status}`);
  return r.json();
}

// ── App polyfill ─────────────────────────────────────────────────────
export const app = {
  Submit: (input: string) => post("/submit", { input }),
  SubmitDisplay: (_d: string, input: string) => post("/submit", { input }),
  Cancel: () => post("/cancel"),
  Approve: (id: string, allow: boolean, session: boolean) => post("/approve", { id, allow, session }),
  AnswerQuestion: (id: string, answers: QuestionAnswer[]) => post("/answer", { id, answers }),
  Compact: () => post("/compact"),
  NewSession: () => post("/new"),
  History: () => get<HistoryMessage[]>("/history"),
  ContextUsage: () => get<ContextInfo>("/context"),
  TCCAReport: async () => { try { return JSON.stringify(await get<unknown>("/tcca-report")); } catch { return ""; } },
  Balance: () => get<BalanceInfo>("/balance"),
  ListSessions: () => get<SessionMeta[]>("/sessions"),
  ResumeSession: (path: string) => post("/resume-session", { path }).then(() => get<HistoryMessage[]>("/history")),
  DeleteSession: (path: string) => post("/delete-session", { path }),
  RenameSession: (path: string, title: string) => post("/rename-session", { path, title }),
  Checkpoints: () => get<CheckpointMeta[]>("/checkpoints"),
  Rewind: (turn: number, scope: string) => post("/checkpoints/rewind", { turn, scope }),
  Fork: (turn: number) => post("/checkpoints/fork", { turn }).then(() => {}),
  SummarizeFrom: (turn: number) => post("/checkpoints/summarize-from", { turn }).then(() => {}),
  SummarizeUpTo: (turn: number) => post("/checkpoints/summarize-up-to", { turn }).then(() => {}),
  Jobs: () => get<JobView[]>("/jobs"),
  Meta: () => get<Meta>("/meta"),
  Models: () => get<ModelInfo[]>("/models"),
  Memory: () => get<MemoryView>("/memory"),
  Remember: (scope: string, note: string) => post("/remember", { scope, note }).then(() => ""),
  Forget: (name: string) => post("/forget", { name }),
  SaveDoc: (path: string, body: string) => post("/save-doc", { path, body }).then(() => ""),
  Commands: () => get<CommandInfo[]>("/commands"),
  Capabilities: () => get<CapabilitiesView>("/capabilities"),
  ListDir: (rel: string) => get<DirEntry[]>(`/files?path=${encodeURIComponent(rel || "")}`),
  ReadFile: (rel: string) => get<FilePreview>(`/file?path=${encodeURIComponent(rel || "")}`),
  Settings: () => get<SettingsView>("/settings"),
  SetBypass: (on: boolean) => post("/settings/bypass", { on }),
  SetModel: (name: string) => post("/settings/model", { ref: name }),
  SetDefaultModel: (ref: string) => post("/settings/default-model", { ref }),
  SaveProvider: (p: ProviderView) => post("/settings/provider", p),
  DeleteProvider: (name: string) => post("/settings/delete-provider", { name }),
  SetProviderKey: (k: string, v: string) => post("/settings/provider-key", { apiKeyEnv: k, value: v }),
  SetAgentParams: (t: number, s: number, p: string) => post("/settings/agent-params", { temperature: t, maxSteps: s, systemPrompt: p }),
  SetSandbox: (bash: string, network: boolean, wr: string, aw: string[]) => post("/settings/sandbox", { bash, network, workspaceRoot: wr, allowWrite: aw }),
  SetPermissionMode: (mode: string) => post("/settings/permission-mode", { mode }),
  AddPermissionRule: (list: string, rule: string) => post("/settings/permission-rule", { list, rule }),
  RemovePermissionRule: (list: string, rule: string) => fetch(`/settings/permission-rule?list=${encodeURIComponent(list)}&rule=${encodeURIComponent(rule)}`, { method: "DELETE", headers: authHeaders() }).then(() => {}),
  AddMCPServer: async (input: MCPServerInput) => { const j = await fetch(`/mcp/add`, { method: "POST", headers: authHeaders(), body: JSON.stringify(input) }).then(r => r.json()); return (j as any).tools as number; },
  RemoveMCPServer: (name: string) => post("/mcp/remove", { name }).then(() => {}),
  RetryMCPServer: (name: string) => post("/mcp/retry", { name }).then(() => {}),
  SetMCPServerEnabled: (name: string, enabled: boolean) => post("/mcp/enabled", { name, enabled }).then(() => {}),
  SlashArgs: (input: string) => get<SlashArgsResult>(`/slash-args?input=${encodeURIComponent(input)}`),
  ListWorkspaces: async () => [] as WorkspaceView[],
  PickWorkspace: async () => "",
  SwitchWorkspace: async () => "",
  OpenWorkspacePath: async () => {},
  RevealWorkspacePath: async () => {},
  SavePastedImage: async () => "",
  AttachmentDataURL: async () => "",
  Version: async () => "1.0.0-mobile",
  CheckUpdate: async () => null,
  ApplyUpdate: async () => {},
  OpenDownloadPage: async () => {},
  MobileAccessStatus: async () => ({ running: false, url: "", publicUrl: "", token: "", port: 0, usingNgrok: false, ngrokReady: false }),
  StartMobileAccess: async () => ({ running: false, url: "", publicUrl: "", token: "", port: 0, usingNgrok: false, ngrokReady: false }),
  StopMobileAccess: async () => {},
  CheckNgrok: async () => false,
  AutoStartMobileAccess: async () => null,
  GetPersistedMobileToken: async () => "",
};
