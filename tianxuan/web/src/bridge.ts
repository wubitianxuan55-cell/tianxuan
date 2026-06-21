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
  AnswerQuestion: async () => {}, // Web MVP: 暂不支持
  SetPlanMode: (on: boolean) => post("/plan", { on }),
  Compact: () => post("/compact"),
  NewSession: () => post("/new"),
  History: () => get<HistoryMessage[]>("/history"),
  ContextUsage: () => get<ContextInfo>("/context"),
  TCCAReport: async () => "",
  Balance: async () => ({ available: false, balance: "", currency: "", message: "Web 版不支持" } as BalanceInfo),

  // 以下为 Web MVP 暂不实现的方法（返回默认值）
  Checkpoints: async () => [] as CheckpointMeta[],
  Rewind: async () => {},
  Fork: async () => {},
  SummarizeFrom: async () => {},
  SummarizeUpTo: async () => {},
  ListSessions: async () => [] as SessionMeta[],
  ResumeSession: async () => [] as HistoryMessage[],
  DeleteSession: async () => {},
  RenameSession: async () => {},
  ListWorkspaces: async () => [] as WorkspaceView[],
  PickWorkspace: async () => "",
  SwitchWorkspace: async () => "",
  Jobs: async () => [] as JobView[],
  Meta: async () => ({ ready: true, startupErr: "", cwd: "", cwdName: "", label: "" } as Meta),
  Commands: async () => [] as CommandInfo[],
  Capabilities: async () => ({ servers: [], skills: [] } as CapabilitiesView),
  AddMCPServer: async () => 0,
  RemoveMCPServer: async () => {},
  RetryMCPServer: async () => {},
  SetMCPServerEnabled: async () => {},
  SlashArgs: async () => ({ args: [] } as SlashArgsResult),
  ListDir: async () => [] as DirEntry[],
  ReadFile: async () => ({ path: "", content: "", truncated: false } as FilePreview),
  OpenWorkspacePath: async () => {},
  RevealWorkspacePath: async () => {},
  SavePastedImage: async () => "",
  AttachmentDataURL: async () => "",
  Models: async () => [] as ModelInfo[],
  SetModel: async () => {},
  Memory: async () => ({ docs: [], facts: [], scopes: [], storeDir: "", available: false } as MemoryView),
  Remember: async () => "",
  Forget: async () => {},
  SaveDoc: async () => "",
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
