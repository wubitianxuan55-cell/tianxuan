// web/src/bridge.ts — HTTP/SSE bridge for the Web UI.
// Implements the same AppBindings interface as desktop/frontend/src/lib/bridge.ts,
// but talks to tianxuan serve (HTTP/SSE) instead of Wails IPC.
//
// Two transports, transparent to callers:
//   Browser → direct fetch + EventSource
//   VS Code → postMessage proxy via extension.ts (CSP-safe)
//
// 缓存安全: 纯传输层，不触及系统提示词/工具Schema/消息链。

import type {
  WireEvent,
  HistoryMessage,
  ContextInfo,
  Meta,
  CapabilitiesView,
  SessionMeta,
  SettingsView,
  MemoryView,
  CheckpointMeta,
  SlashArgsResult,
  DirEntry,
  FilePreview,
  WorkspaceView,
  BalanceInfo,
  JobView,
  CommandInfo,
  QuestionAnswer,
  MCPServerInput,
  ModelInfo,
  ProviderView,
  UpdateInfo,
} from "@shared/lib/types";

// ── VS Code 环境检测 ─────────────────────────────────────────────────

interface VSCodeAPI {
  postMessage(msg: unknown): void;
  getState(): unknown;
  setState(state: unknown): void;
}
declare function acquireVsCodeApi(): VSCodeAPI;

const isVSCode = typeof acquireVsCodeApi !== "undefined";

// ── VS Code postMessage 请求器 ───────────────────────────────────────

type PendingReq = { resolve: (v: unknown) => void; reject: (e: Error) => void };

function makeVSCodeProxy() {
  const api = acquireVsCodeApi();
  let reqId = 0;
  const pending = new Map<number, PendingReq>();

  window.addEventListener("message", (ev: MessageEvent) => {
    const msg = ev.data;
    if (!msg || typeof msg !== "object") return;

    // 普通请求/响应（HTTP 代理 + 原生 API）
    if (msg.type === "tianxuan:response") {
      const p = pending.get(msg.id as number);
      if (p) { pending.delete(msg.id as number); handleResponse(p, msg); }
    }

    // SSE 事件转发
    if (msg.type === "tianxuan:sse:event") {
      try {
        const e = JSON.parse(msg.data as string) as WireEvent;
        listeners.forEach((l) => l(e));
      } catch { /* ignore */ }
    }
  });

  function handleResponse(p: PendingReq, msg: Record<string, unknown>) {
    if (msg.error) p.reject(new Error(msg.error as string));
    else p.resolve(msg.result);
  }

  function request(method: string, params?: Record<string, unknown>): Promise<unknown> {
    return new Promise((resolve, reject) => {
      const id = ++reqId;
      pending.set(id, { resolve, reject });
      api.postMessage({ type: "tianxuan:request", id, method, params });
      setTimeout(() => { if (pending.delete(id)) reject(new Error("timeout")); }, 30000);
    });
  }

  // HTTP 代理 — 把 fetch 调用包装成 postMessage 请求
  async function proxyFetch(m: string, path: string, body?: unknown): Promise<{ status: number; body: string }> {
    return await request("fetch", { method: m, path, body }) as { status: number; body: string };
  }

  return { request, proxyFetch };
}

const vscodeProxy = isVSCode ? makeVSCodeProxy() : null;

// ── SSE (两套传输) ───────────────────────────────────────────────────

const listeners = new Set<(e: WireEvent) => void>();
let sseConnected = false;

export function onEvent(cb: (e: WireEvent) => void): () => void {
  listeners.add(cb);
  if (!sseConnected) {
    sseConnected = true;
    if (isVSCode) {
      vscodeProxy!.request("sse:connect");
    } else {
      const es = new EventSource("/events");
      es.onmessage = (msg) => {
        try { listeners.forEach((l) => l(JSON.parse(msg.data) as WireEvent)); } catch { /* */ }
      };
      es.onerror = () => { /* auto-reconnect */ };
    }
  }
  return () => listeners.delete(cb);
}

// ── HTTP helpers (两套传输) ──────────────────────────────────────────

async function post(path: string, body?: unknown): Promise<void> {
  if (isVSCode) {
    const r = await vscodeProxy!.proxyFetch("POST", path, body);
    if (r.status >= 400) throw new Error(`${r.status}`);
    return;
  }
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
}

async function get<T>(path: string): Promise<T> {
  if (isVSCode) {
    const r = await vscodeProxy!.proxyFetch("GET", path);
    if (r.status >= 400) throw new Error(`${r.status}`);
    return JSON.parse(r.body) as T;
  }
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

async function fetchJson<T>(path: string, method: string, body?: unknown): Promise<T> {
  if (isVSCode) {
    const r = await vscodeProxy!.proxyFetch(method, path, body);
    if (r.status >= 400) throw new Error(`${r.status}`);
    return JSON.parse(r.body) as T;
  }
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

// ── AppBindings 实现 ─────────────────────────────────────────────────

export const app = {
  Submit: (input: string) => post("/submit", { input }),
  SubmitDisplay: (_display: string, input: string) => post("/submit", { input }),
  Cancel: () => post("/cancel"),
  Approve: (id: string, allow: boolean, session: boolean) => post("/approve", { id, allow, session }),
  AnswerQuestion: (id: string, answers: QuestionAnswer[]) => post("/answer", { id, answers }),
  Compact: () => post("/compact"),
  Compact: () => post("/compact"),
  NewSession: () => post("/new"),
  History: () => get<HistoryMessage[]>("/history"),
  ContextUsage: () => get<ContextInfo>("/context"),
  TCCAReport: async () => {
    try { return JSON.stringify(await get<unknown>("/tcca-report")); } catch { return ""; }
  },
  Balance: () => get<BalanceInfo>("/balance"),

  // sessions
  ListSessions: () => get<SessionMeta[]>("/sessions"),
  ResumeSession: (path: string) => post("/resume-session", { path }).then(() => get<HistoryMessage[]>("/history")),
  DeleteSession: (path: string) => post("/delete-session", { path }),
  RenameSession: (path: string, title: string) => post("/rename-session", { path, title }),

  // checkpoints
  Checkpoints: () => get<CheckpointMeta[]>("/checkpoints"),
  Rewind: (turn: number, scope: string) => post("/checkpoints/rewind", { turn, scope }),
  Fork: (turn: number) => post("/checkpoints/fork", { turn }).then(() => {}),
  SummarizeFrom: (turn: number) => post("/checkpoints/summarize-from", { turn }).then(() => {}),
  SummarizeUpTo: (turn: number) => post("/checkpoints/summarize-up-to", { turn }).then(() => {}),

  // jobs / meta / models
  Jobs: () => get<JobView[]>("/jobs"),
  Meta: () => get<Meta>("/meta"),
  Models: () => get<ModelInfo[]>("/models"),

  // memory
  Memory: () => get<MemoryView>("/memory"),
  Remember: (scope: string, note: string) => post("/remember", { scope, note }).then(() => ""),
  Forget: (name: string) => post("/forget", { name }),
  SaveDoc: (path: string, body: string) => post("/save-doc", { path, body }).then(() => ""),

  // commands / capabilities
  Commands: () => get<CommandInfo[]>("/commands"),
  Capabilities: () => get<CapabilitiesView>("/capabilities"),

  // files
  ListDir: (rel: string) => get<DirEntry[]>(`/files?path=${encodeURIComponent(rel || "")}`),
  ReadFile: (rel: string) => get<FilePreview>(`/file?path=${encodeURIComponent(rel || "")}`),

  // settings
  Settings: () => get<SettingsView>("/settings"),
  SetBypass: (on: boolean) => post("/settings/bypass", { on }),
  SetModel: (name: string) => post("/settings/model", { ref: name }),
  SetDefaultModel: (ref: string) => post("/settings/default-model", { ref }),
  SaveProvider: (p: ProviderView) => post("/settings/provider", p),
  DeleteProvider: (name: string) => post("/settings/delete-provider", { name }),
  SetProviderKey: (apiKeyEnv: string, value: string) => post("/settings/provider-key", { apiKeyEnv, value }),
  SetAgentParams: (temperature: number, maxSteps: number, systemPrompt: string) =>
    post("/settings/agent-params", { temperature, maxSteps, systemPrompt }),
  SetSandbox: (bash: string, network: boolean, workspaceRoot: string, allowWrite: string[]) =>
    post("/settings/sandbox", { bash, network, workspaceRoot, allowWrite }),
  SetPermissionMode: (mode: string) => post("/settings/permission-mode", { mode }),
  AddPermissionRule: (list: string, rule: string) => post("/settings/permission-rule", { list, rule }),
  RemovePermissionRule: (list: string, rule: string) =>
    isVSCode
      ? fetchJson("/settings/permission-rule", "DELETE", { list, rule }).then(() => {})
      : fetch(`/settings/permission-rule?list=${encodeURIComponent(list)}&rule=${encodeURIComponent(rule)}`, { method: "DELETE" }).then(() => {}),

  // MCP
  AddMCPServer: async (input: MCPServerInput) => {
    const j = await fetchJson<{ tools: number }>("/mcp/add", "POST", input);
    return j.tools;
  },
  RemoveMCPServer: (name: string) => post("/mcp/remove", { name }).then(() => {}),
  RetryMCPServer: (name: string) => post("/mcp/retry", { name }).then(() => {}),
  SetMCPServerEnabled: (name: string, enabled: boolean) => post("/mcp/enabled", { name, enabled }).then(() => {}),

  // slash
  SlashArgs: (input: string) => get<SlashArgsResult>(`/slash-args?input=${encodeURIComponent(input)}`),

  // workspace / desktop file ops — postMessage in VS Code, no-op in browser
  ListWorkspaces: async () => isVSCode ? await vscodeProxy!.request("listWorkspaces") as WorkspaceView[] : [],
  PickWorkspace: async () => isVSCode ? await vscodeProxy!.request("pickWorkspace") as string : "",
  SwitchWorkspace: async (path: string) => isVSCode ? await vscodeProxy!.request("switchWorkspace", { path }) as string : "",
  OpenWorkspacePath: async (rel: string) => { if (isVSCode) await vscodeProxy!.request("openWorkspacePath", { rel }); },
  RevealWorkspacePath: async (rel: string) => { if (isVSCode) await vscodeProxy!.request("revealWorkspacePath", { rel }); },
  SavePastedImage: async () => "",
  AttachmentDataURL: async () => "",

  // updates
  Version: async () => isVSCode
    ? (() => { try { return vscodeProxy!.request("version") as Promise<string>; } catch { return "8.12.0-vscode"; } })()
    : "8.12.0-web",
  CheckUpdate: async () => null,
  ApplyUpdate: async () => {},
  OpenDownloadPage: async () => {},
  // Mobile
  MobileAccessStatus: async () => ({ running: false, url: "", publicUrl: "", token: "", port: 0, usingNgrok: false, ngrokReady: false }),
  StartMobileAccess: async () => ({ running: false, url: "", publicUrl: "", token: "", port: 0, usingNgrok: false, ngrokReady: false }),
  StopMobileAccess: async () => {},
  CheckNgrok: async () => false,
  AutoStartMobileAccess: async () => null,
  GetPersistedMobileToken: async () => "",
};
