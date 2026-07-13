// bridge is the single seam between the React app and the Go kernel. In the Wails
// shell it calls the bound App methods (window.go.main.App.*) and subscribes to
// the runtime event stream (window.runtime.EventsOn). In a plain browser (`pnpm
// dev` outside the shell) those globals are absent, so it falls back to a mock
// that streams a canned turn through the same contract — letting the whole UI be
// developed and laid out without rebuilding the Go side.

import type {
  BalanceInfo,
  WorkspaceChangeView,
  CapabilitiesView,
  CheckpointMeta,
  CommandInfo,
  ContextInfo,
  DirEntry,
  FilePreview,
  HistoryMessage,
  JobView,
  MCPServerInput,
  MemoryView,
  MemorySuggestion,
  MemorySuggestionsView,
  ResultView,
  ScheduleView,
  SkillSuggestion,
  TabMeta,
  Meta,
  ModelInfo,
  ProviderView,
  QuestionAnswer,
  SessionMeta,
  SettingsView,
  SlashArgsResult,
  UpdateInfo,
  UpdateProgress,
  WireEvent,
  WorkspaceView,
} from "./types";

// AppBindings mirrors desktop/app.go's exported method set. Keep in sync by hand
// (or regenerate with `wails generate module` and import wailsjs instead).
//
// Compile-time drift check: when a Go method is added/renamed but AppBindings is
// not updated, the type assertion below catches it at build time.  Fix: add the
// missing method to AppBindings, then run `pnpm typecheck`.
export interface AppBindings {
  Submit(input: string): Promise<void>;
  SubmitDisplay(display: string, input: string): Promise<void>;
  Cancel(): Promise<void>;
  Approve(id: string, allow: boolean, session: boolean): Promise<void>;
  AnswerQuestion(id: string, answers: QuestionAnswer[]): Promise<void>;
  SetAgentMode(mode: string): Promise<void>;
  SetAgentMode(mode: string): Promise<void>;
  AgentMode(): Promise<string>;
  Compact(): Promise<void>;
  NewSession(): Promise<void>;
  History(): Promise<HistoryMessage[]>;
  // Checkpoints lists the session's rewind points; Rewind restores one (scope
  // "code" | "conversation" | "both"), after which the caller re-reads History.
  Checkpoints(): Promise<CheckpointMeta[]>;
  Rewind(turn: number, scope: string): Promise<void>;
  Fork(turn: number): Promise<void>;
  SummarizeFrom(turn: number): Promise<void>;
  SummarizeUpTo(turn: number): Promise<void>;
  // Session history: list saved sessions, resume one (returns its transcript),
  // delete one, or give one a custom display name ("" clears it).
  ListSessions(): Promise<SessionMeta[]>;
  ResumeSession(path: string): Promise<HistoryMessage[]>;
  DeleteSession(path: string): Promise<void>;
  RenameSession(path: string, title: string): Promise<void>;
  // Workspace: open a folder chooser and switch to that project (fresh session);
  // returns the chosen path, or "" if cancelled.
  ListWorkspaces(): Promise<WorkspaceView[]>;
  PickWorkspace(): Promise<string>;
  SwitchWorkspace(path: string): Promise<string>;
  ContextUsage(): Promise<ContextInfo>;
  // TCCA 缓存报告（V3.0）— 返回 CacheReport JSON 字符串
  TCCAReport(): Promise<string>;
  // Balance queries the active provider's wallet balance (a network call);
  // returns an unavailable readout when no balance_url is configured or it fails.
  Balance(): Promise<BalanceInfo>;
  // Jobs lists the running background jobs (bash/task started in the background)
  // for the status-bar indicator.
  Jobs(): Promise<JobView[]>;
  Meta(): Promise<Meta>;
  Commands(): Promise<CommandInfo[]>;
  // Capabilities feeds the MCP & Skills drawer: connected/failed servers + skills.
  // Add connects + persists a server; Remove disconnects + drops it from config;
  // Retry reconnects a configured server that failed (config untouched).
  Capabilities(): Promise<CapabilitiesView>;
  AddMCPServer(input: MCPServerInput): Promise<number>;
  RemoveMCPServer(name: string): Promise<void>;
  RetryMCPServer(name: string): Promise<void>;
  // SetMCPServerEnabled is the per-session connector toggle (on reconnects, off
  // disconnects; config untouched).
  SetMCPServerEnabled(name: string, enabled: boolean): Promise<void>;
  SlashArgs(input: string): Promise<SlashArgsResult>;
  ListDir(rel: string): Promise<DirEntry[]>;
  ReadFile(rel: string): Promise<FilePreview>;
  OpenWorkspacePath(rel: string): Promise<void>;
  // WorkspaceChanges returns files modified during this session by the agent.
  WorkspaceChanges(): Promise<WorkspaceChangeView[]>;
  RevealWorkspacePath(rel: string): Promise<void>;
  SavePastedImage(dataUrl: string): Promise<string>;
  AttachmentDataURL(path: string): Promise<string>;
  Models(): Promise<ModelInfo[]>;
  SetModel(name: string): Promise<void>;
  // Memory panel: read the loaded REASONIX.md hierarchy + saved auto-memories,
  // quick-add a note to a scope's REASONIX.md (≡ "#<note>"), and overwrite a doc
  // from the in-place editor.
  Memory(): Promise<MemoryView>;
  MemoryForTab(tabID: string): Promise<MemoryView>;
  Remember(scope: string, note: string): Promise<string>;
  Forget(name: string): Promise<void>;
  ForgetForTab(tabID: string, name: string): Promise<void>;
  SaveDoc(path: string, body: string): Promise<string>;
  UpdateFact(name: string, body: string): Promise<string>;
  ChangeFactType(name: string, type: string): Promise<string>;
  MemorySuggestions(): Promise<MemorySuggestionsView>;
  MemorySuggestionsForTab(tabID: string): Promise<MemorySuggestionsView>;
  AcceptMemorySuggestion(candidate: MemorySuggestion): Promise<string>;
  AcceptSkillSuggestion(candidate: SkillSuggestion): Promise<string>;
  SelectTab(tabID: string): Promise<void>;
  TabMeta(): Promise<TabMeta[]>;
  // Settings panel: read the resolved config and apply edits (each writes config
  // and rebuilds the controller live). Secrets go through SetProviderKey (→ .env).
  Settings(): Promise<SettingsView>;
  SetDefaultModel(ref: string): Promise<void>;
  SaveProvider(p: ProviderView): Promise<void>;
  DeleteProvider(name: string): Promise<void>;
  SetProviderKey(apiKeyEnv: string, value: string): Promise<void>;
  SetPermissionMode(mode: string): Promise<void>;
  AddPermissionRule(list: string, rule: string): Promise<void>;
  RemovePermissionRule(list: string, rule: string): Promise<void>;
  SetSandbox(bash: string, network: boolean, workspaceRoot: string, allowWrite: string[]): Promise<void>;
  SetAgentParams(temperature: number, maxSteps: number, systemPrompt: string): Promise<void>;
  // SetPlannerTemperature sets the planner-specific temperature override.
  // 0 means "use the global temperature".
  SetPlannerTemperature(temp: number): Promise<void>;
  // SetSubagentTemperature sets the subagent-specific temperature override.
  // 0 means "use the global temperature".
  SetSubagentTemperature(temp: number): Promise<void>;
  // SetEffort sets the reasoning effort for the executor. "" = provider default.
  SetEffort(effort: string): Promise<void>;
  // SetPlannerEffort sets the reasoning effort for Hermes. "" = inherit from Effort.
  SetPlannerEffort(effort: string): Promise<void>;
  // SetSubagentEffort sets the reasoning effort for sub-agents. "" = inherit from Effort.
  SetSubagentEffort(effort: string): Promise<void>;
  // SetSubagentModel sets the default model for spawned sub-agents. An empty string
  // clears it so sub-agents inherit the parent's provider.
  SetSubagentModel(ref: string): Promise<void>;
  // SetSubagentModelForSkill sets a per-skill sub-agent model override.
  // skill is one of explore|research|review|security-review. Empty ref = inherit.
  SetSubagentModelForSkill(skill: string, ref: string): Promise<void>;
  // SetPlannerModel sets (or, with "", clears) the two-model planner.
  SetPlannerModel(ref: string): Promise<void>;
  // SetPermLevel controls permission strictness: "ask" (default, prompt before writes),
  // "auto" (allow writes without asking), or "yolo" (skip all prompts).
  SetPermLevel(level: string): Promise<void>;
  // Auto-updater (desktop/updater_app.go): the injected build version, a manifest
  // check, applying an update (win/linux self-update; macOS opens the download
  // page), and opening that page directly. Progress streams on "updater:progress".
  Version(): Promise<string>;
  CheckUpdate(): Promise<UpdateInfo | null>;
  ApplyUpdate(): Promise<void>;
  OpenDownloadPage(): Promise<void>;
  // Window state persistence.
  SaveWindowState(state: {width:number;height:number;x:number;y:number;maximised:boolean}): Promise<void>;
  // Schedule management.
  GetSchedules(): Promise<ScheduleView[]>;
  CreateSchedule(v: ScheduleView): Promise<ScheduleView>;
  UpdateSchedule(v: ScheduleView): Promise<void>;
  DeleteSchedule(id: string): Promise<void>;
  ToggleSchedule(id: string, enabled: boolean): Promise<void>;
  RunScheduleNow(id: string): Promise<ResultView>;
  GetResults(scheduleID: string): Promise<ResultView[]>;
  RefineSchedulePrompt(prompt: string): Promise<string>;
}

interface WailsRuntime {
  EventsOn(name: string, cb: (...data: unknown[]) => void): () => void;
  BrowserOpenURL(url: string): void;
}

declare global {
  interface Window {
    runtime?: WailsRuntime;
    go?: { main?: { App?: AppBindings } };
  }
}

// Must match desktop/app.go's eventChannel constant.
const EVENT_CHANNEL = "agent:event";

// Resolve the Wails binding at CALL time, not module-load time: in dev the Wails
// runtime can inject window.go AFTER this module first evaluates, so snapshotting
// once would pin the browser mock for the whole session (and show fake data — the
// dev mock's model list leaking into the real app was exactly this bug).
function realApp(): AppBindings | undefined {
  return typeof window !== "undefined" ? window.go?.main?.App : undefined;
}

let mockSingleton: AppBindings | null = null;
function getMock(): AppBindings {
  if (!mockSingleton) mockSingleton = makeMockApp();
  return mockSingleton;
}

// onEvent subscribes to the agent's typed event stream; returns an unsubscribe.
export function onEvent(cb: (e: WireEvent) => void): () => void {
  if (realApp() && typeof window !== "undefined" && window.runtime) {
    return window.runtime.EventsOn(EVENT_CHANNEL, (payload) => cb(payload as WireEvent));
  }
  return mockSubscribe(cb);
}

// onUpdaterProgress subscribes to the auto-updater's progress events (a separate
// channel from the agent stream); returns an unsubscribe. Must match the event
// name emitted in desktop/updater_app.go.
export function onUpdaterProgress(cb: (p: UpdateProgress) => void): () => void {
  if (realApp() && typeof window !== "undefined" && window.runtime) {
    return window.runtime.EventsOn("updater:progress", (p) => cb(p as UpdateProgress));
  }
  updaterListeners.add(cb);
  return () => {
    updaterListeners.delete(cb);
  };
}

// onReady subscribes to the agent:ready event fired when boot.Build completes.
// The frontend re-fetches Meta/Context/History when this lands.
export function onReady(cb: () => void): () => void {
  if (realApp() && typeof window !== "undefined" && window.runtime) {
    return window.runtime.EventsOn("agent:ready", () => cb());
  }
  // In dev mock, fire immediately since there's no real boot sequence.
  cb();
  return () => {};
}

// app proxies each call to the live binding (or the dev mock only when truly
// outside the shell), so a late-injected window.go is picked up transparently.
export const app: AppBindings = new Proxy({} as AppBindings, {
  get(_t, prop) {
    const target = realApp() ?? getMock();
    const v = (target as unknown as Record<string, unknown>)[String(prop)];
    return typeof v === "function" ? (v as (...a: unknown[]) => unknown).bind(target) : v;
  },
});

// openExternal opens a URL in the system browser (so links in rendered markdown
// don't navigate the webview away from the app). Falls back to window.open in the
// browser dev mock.
export function openExternal(url: string): void {
  if (typeof window !== "undefined" && window.runtime?.BrowserOpenURL) {
    window.runtime.BrowserOpenURL(url);
  } else if (typeof window !== "undefined") {
    window.open(url, "_blank", "noopener");
  }
}

import {
  makeMockApp,
  mockSubscribe,
  updaterListeners,
} from "./mock";

// ── compile-time drift check ──────────────────────────────────────────────
// _CheckGenToApp errors when a generated Go method has no TS counterpart in
// AppBindings. Fix: add the missing method to AppBindings, then `pnpm typecheck`.
// Methods intentionally excluded from the frontend can be listed in the Exclude
// union to silence the check.
import type * as GeneratedApp from "../../wailsjs/go/main/App";

type AssertNever<T extends never> = T;
export type _CheckGenToApp = AssertNever<Exclude<keyof typeof GeneratedApp, keyof AppBindings | "QuitApp" | "ShowWindow" | "SetBypass" | "SetAgentMode" | "PermLevel" | "StartMobileAccess" | "StopMobileAccess" | "MobileAccessStatus" | "CheckNgrok" | "AutoStartMobileAccess" | "GetPersistedMobileToken">>;
