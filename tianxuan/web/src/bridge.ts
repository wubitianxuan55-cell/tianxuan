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
  TCCAReport: async () => "",
  Balance: () => get<BalanceInfo>("/balance"),

  // ── sessions ──
  ListSessions: () => get<SessionMeta[]>("/sessions"),
  ResumeSession: (path: string) => post("/resume-session", { path }).then(() => get<HistoryMessage[]>("/history")),
  DeleteSession: (path: string) => post("/delete-session", { path }),

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

  // 以下暂不实现
  Checkpoints: async () => [] as CheckpointMeta[],
  ListWorkspaces: async () => [] as WorkspaceView[],
  PickWorkspace: async () => "",
  SwitchWorkspace: async () => "",
  OpenWorkspacePath: async () => {},
  RevealWorkspacePath: async () => {},
  SavePastedImage: async () => "",
  AttachmentDataURL: async () => "",
  SlashArgs: async () => ({ items: [], from: 0, total: 0 } as SlashArgsResult),
  AddMCPServer: async () => 0,
  RemoveMCPServer: async () => {},
  RetryMCPServer: async () => {},
  SetMCPServerEnabled: async () => {},
  SetModel: async () => {},
  Settings: async () => ({} as SettingsView),
  SetDefaultModel: async () => {},
  SaveProvider: async () => {},
  DeleteProvider: async () => {},
  SetProviderKey: async () => {},
  SetPermissionMode: async () => {},
  AddPermissionRule: async () => {},
  RemovePermissionRule: async () => {},
  SetSandbox: async () => {},
  SetAgentParams: async () => {},
  SetBypass: async () => {},
  Version: async () => "8.6.0-web",
  CheckUpdate: async () => null,
  ApplyUpdate: async () => {},
  OpenDownloadPage: async () => {},
};
