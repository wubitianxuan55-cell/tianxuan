// web/src/bridge.ts — HTTP/SSE bridge for the Web UI.
// Implements the same AppBindings interface as desktop/frontend/src/lib/bridge.ts,
// but talks to tianxuan serve (HTTP/SSE) instead of Wails IPC.
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

// ── SSE event subscription ──

let eventSource: EventSource | null = null;
const listeners = new Set<(e: WireEvent) => void>();

export function onEvent(cb: (e: WireEvent) => void): () => void {
  listeners.add(cb);
  if (!eventSource) {
    eventSource = new EventSource("/events");
    eventSource.onmessage = (msg) => {
      try {
        const e = JSON.parse(msg.data) as WireEvent;
        listeners.forEach((l) => l(e));
      } catch { /* ignore parse errors */ }
    };
    eventSource.onerror = () => {
      // Auto-reconnect handled by EventSource
    };
  }
  return () => listeners.delete(cb);
}

// ── HTTP helpers ──

async function post(path: string, body?: unknown): Promise<void> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

// ── AppBindings 实现 ──

export const app = {
  Submit: (input: string) => post("/submit", { input }),
  SubmitDisplay: (_display: string, input: string) => post("/submit", { input }),
  Cancel: () => post("/cancel"),
  Approve: (id: string, allow: boolean, session: boolean) => post("/approve", { id, allow, session }),
  AnswerQuestion: (id: string, answers: QuestionAnswer[]) => post("/answer", { id, answers }),
  SetPlanMode: (on: boolean) => post("/plan", { on }),
  Compact: () => post("/compact"),
  NewSession: () => post("/new"),
  History: () => get<HistoryMessage[]>("/history"),
  ContextUsage: () => get<ContextInfo>("/context"),
  TCCAReport: async () => {
    try { return JSON.stringify(await get<unknown>("/tcca-report")); } catch { return ""; }
  },
  Balance: () => get<BalanceInfo>("/balance"),

  // ── sessions ──
  ListSessions: () => get<SessionMeta[]>("/sessions"),
  ResumeSession: (path: string) => post("/resume-session", { path }).then(() => get<HistoryMessage[]>("/history")),
  DeleteSession: (path: string) => post("/delete-session", { path }),
  RenameSession: (path: string, title: string) => post("/rename-session", { path, title }),

  // ── checkpoints ──
  Checkpoints: () => get<CheckpointMeta[]>("/checkpoints"),
  Rewind: (turn: number, scope: string) => post("/checkpoints/rewind", { turn, scope }),
  Fork: (turn: number) => post("/checkpoints/fork", { turn }).then(() => {}),
  SummarizeFrom: (turn: number) => post("/checkpoints/summarize-from", { turn }).then(() => {}),
  SummarizeUpTo: (turn: number) => post("/checkpoints/summarize-up-to", { turn }).then(() => {}),

  // ── jobs ──
  Jobs: () => get<JobView[]>("/jobs"),

  // ── meta ──
  Meta: () => get<Meta>("/meta"),

  // ── models ──
  Models: () => get<ModelInfo[]>("/models"),

  // ── memory ──
  Memory: () => get<MemoryView>("/memory"),
  Remember: (scope: string, note: string) => post("/remember", { scope, note }).then(() => ""),
  Forget: (name: string) => post("/forget", { name }),
  SaveDoc: (path: string, body: string) => post("/save-doc", { path, body }).then(() => ""),

  // ── commands / capabilities ──
  Commands: () => get<CommandInfo[]>("/commands"),
  Capabilities: () => get<CapabilitiesView>("/capabilities"),

  // ── files ──
  ListDir: (rel: string) => get<DirEntry[]>(`/files?path=${encodeURIComponent(rel || "")}`),
  ReadFile: (rel: string) => get<FilePreview>(`/file?path=${encodeURIComponent(rel || "")}`),

  // ── settings ──
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
    fetch(`/settings/permission-rule?list=${encodeURIComponent(list)}&rule=${encodeURIComponent(rule)}`, { method: "DELETE" }).then(() => {}),

  // ── MCP ──
  AddMCPServer: async (input: MCPServerInput) => {
    const res = await fetch("/mcp/add", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) throw new Error(`${res.status}`);
    const j = await res.json() as { tools: number };
    return j.tools;
  },
  RemoveMCPServer: (name: string) => post("/mcp/remove", { name }).then(() => {}),
  RetryMCPServer: (name: string) => post("/mcp/retry", { name }).then(() => {}),
  SetMCPServerEnabled: (name: string, enabled: boolean) => post("/mcp/enabled", { name, enabled }).then(() => {}),

  // ── slash ──
  SlashArgs: (input: string) => get<SlashArgsResult>(`/slash-args?input=${encodeURIComponent(input)}`),

// ── workspace / desktop file ops (no-op on web, postMessage in VS Code) ──
  ListWorkspaces: async () => {
    if (vscode) {
      return await vscode.request("listWorkspaces") as WorkspaceView[];
    }
    return [] as WorkspaceView[];
  },
  PickWorkspace: async () => {
    if (vscode) {
      return await vscode.request("pickWorkspace") as string;
    }
    return "";
  },
  SwitchWorkspace: async (path: string) => {
    if (vscode) {
      return await vscode.request("switchWorkspace", { path }) as string;
    }
    return "";
  },
  OpenWorkspacePath: async (rel: string) => {
    if (vscode) {
      await vscode.request("openWorkspacePath", { rel });
    }
  },
  RevealWorkspacePath: async (rel: string) => {
    if (vscode) {
      await vscode.request("revealWorkspacePath", { rel });
    }
  },
  SavePastedImage: async () => "",
  AttachmentDataURL: async () => "",

  // ── updates (no-op on web) ──
  Version: async () => {
    if (vscode) {
      try { return await vscode.request("version") as string; } catch { return "8.10.0-vscode"; }
    }
    return "8.10.0-web";
  },
  CheckUpdate: async () => null,
  ApplyUpdate: async () => {},
  OpenDownloadPage: async () => {},
};

// ── VS Code 桥接层 ──
// 当 web 前端运行在 VS Code Webview 内时，通过 postMessage 调用
// VS Code 原生 API（工作区选择器、文件打开等）。在普通浏览器中
// vscode 为 null，所有操作 fallback 到 no-op。

interface VSCodeAPI {
  postMessage(msg: unknown): void;
  getState(): unknown;
  setState(state: unknown): void;
}

declare function acquireVsCodeApi(): VSCodeAPI;

const vscode: {
  request(method: string, params?: Record<string, unknown>): Promise<unknown>;
} | null = (() => {
  if (typeof acquireVsCodeApi === "undefined") return null;

  const api = acquireVsCodeApi();
  let reqId = 0;
  const pending = new Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();

  window.addEventListener("message", (ev: MessageEvent) => {
    const msg = ev.data;
    if (msg && typeof msg === "object" && msg.type === "tianxuan:response") {
      const p = pending.get(msg.id as number);
      if (p) {
        pending.delete(msg.id as number);
        if (msg.error) p.reject(new Error(msg.error as string));
        else p.resolve(msg.result);
      }
    }
  });

  return {
    request(method: string, params?: Record<string, unknown>): Promise<unknown> {
      return new Promise((resolve, reject) => {
        const id = ++reqId;
        pending.set(id, { resolve, reject });
        api.postMessage({ type: "tianxuan:request", id, method, params });
        setTimeout(() => {
          if (pending.delete(id)) reject(new Error("timeout"));
        }, 30000);
      });
    },
  };
})();
