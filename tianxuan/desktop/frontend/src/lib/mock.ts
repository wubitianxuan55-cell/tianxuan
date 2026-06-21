// bridge/mock.ts — 浏览器开发模式的 mock 实现。
// 仅在 Wails 环境不可用时加载（pnpm dev 模式），
// 模拟 tianxuan 后端的响应，让整个 UI 可独立开发调试。
//
// 缓存安全: 纯前端 mock，不触及 Go 内核。

import type {
  MCPServerInput,
  Meta,
  ProviderView,
  ServerView,
  SessionMeta,
  SettingsView,
  SkillView,
  UpdateProgress,
  WireEvent,
} from "./types";
import type { AppBindings } from "./bridge";

const EVENT_CHANNEL = "agent:event";

export const mockListeners = new Set<(e: WireEvent) => void>();

export function mockSubscribe(cb: (e: WireEvent) => void): () => void {
  mockListeners.add(cb);
  return () => {
    mockListeners.delete(cb);
  };
}

export function emitMock(e: WireEvent) {
  mockListeners.forEach((l) => l(e));
}

// 内部别名 — makeMockApp 内部用 emit() 调用
const emit = emitMock;

export const updaterListeners = new Set<(p: UpdateProgress) => void>();

function emitUpdater(p: UpdateProgress) {
  updaterListeners.forEach((l) => l(p));
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

export function makeMockApp(): AppBindings {
  let cancelled = false;
  let cwd = "~/projects/tianxuan"; // mutable so PickWorkspace is visible in dev
  let workspaces = ["~/projects/tianxuan", "~/projects/blade", "~/projects/deepseek-forge", "~/projects/cc-switch-light", "~/projects/SuperRig"];
  const day = 86_400_000;
  const t0 = Date.now();
  // Mutable so MCP add/remove/retry are observable in browser dev.
  let capServers: ServerView[] = [
    {
      name: "codegraph",
      transport: "stdio",
      status: "connected",
      tools: 4,
      prompts: 0,
      resources: 1,
      toolList: [
        { name: "search", description: "Search symbols, files, and text in the workspace." },
        { name: "context", description: "Fetch surrounding source context for a symbol or file." },
        { name: "trace", description: "Follow callers and callees across the code graph." },
        { name: "node", description: "Inspect a specific graph node." },
      ],
    },
    { name: "github", transport: "stdio", status: "connected", tools: 12, prompts: 2, resources: 0 },
    { name: "linear", transport: "http", status: "connected", tools: 8, prompts: 0, resources: 0 },
    { name: "figma", transport: "http", status: "failed", tools: 0, prompts: 0, resources: 0, error: "connect: 401 unauthorized" },
  ];
  const capSkills: SkillView[] = [
    { name: "explore", description: "Investigate the codebase in an isolated subagent", scope: "builtin", runAs: "subagent" },
    { name: "review", description: "Review the staged diff", scope: "project", runAs: "inline" },
    { name: "init", description: "Scaffold a REASONIX.md for this repo", scope: "builtin", runAs: "inline" },
  ];
  const mockSwitchWorkspace = async (path: string) => {
    cwd = path || "~";
    workspaces = [cwd, ...workspaces.filter((p) => p !== cwd)].slice(0, 12);
    return cwd;
  };
  // Mutable so delete/rename are observable in browser dev.
  const sessions: SessionMeta[] = [
    { path: "/mock/sessions/a.jsonl", preview: "fix the login bug in auth.go", turns: 12, modTime: t0 - 3_600_000, current: true },
    { path: "/mock/sessions/b.jsonl", preview: "refactor the payment module", turns: 5, modTime: t0 - 6 * 3_600_000, current: false },
    { path: "/mock/sessions/c.jsonl", preview: "write the README and badges", turns: 8, modTime: t0 - day - 3_600_000, current: false },
    { path: "/mock/sessions/d.jsonl", preview: "explain the plugin host design", turns: 3, modTime: t0 - 4 * day, current: false },
  ];
  // Mutable settings so the Settings panel's edits are observable in browser dev.
  const settings: SettingsView = {
    defaultModel: "deepseek-flash",
    providers: [
      { name: "deepseek-flash", kind: "openai", baseUrl: "https://api.deepseek.com", models: ["deepseek-v4-flash"], default: "deepseek-v4-flash", apiKeyEnv: "DEEPSEEK_API_KEY", keySet: true, balanceUrl: "https://api.deepseek.com/user/balance", contextWindow: 1_000_000 },
      { name: "mimo-pro", kind: "openai", baseUrl: "https://api.xiaomimimo.com/v1", models: ["mimo-v2.5-pro"], default: "mimo-v2.5-pro", apiKeyEnv: "MIMO_API_KEY", keySet: false, balanceUrl: "", contextWindow: 1_000_000 },
    ],
    permissions: { mode: "ask", allow: ["ls", "read_file"], ask: [], deny: ["bash(rm *)"] },
    sandbox: { bash: "enforce", network: true, workspaceRoot: "", allowWrite: [] },
    agent: { temperature: 0.2, maxSteps: 0, systemPrompt: "You are tianxuan, a coding agent." },
    configPath: "~/projects/tianxuan/tianxuan.toml",
    providerKinds: ["openai"],
    bypass: false,
  };
  return {
    async Submit(input) {
      cancelled = false;
      emit({ kind: "turn_started" });
      await delay(300);
      if (cancelled) return;
      const isPoetry = /(诗|古诗|词)/.test(input);
      const isCodeReq = !isPoetry && /(写|创建|程序|代码|函数|排序)/.test(input);
      const think = isPoetry ? "用户想写诗，直接创作即可。"
        : isCodeReq ? `用户说"${input}"，这是编程任务，先检查项目结构。`
        : `用户说"${input}"，让我看看上下文再回复。`;
      for (const ch of think) { if (cancelled) break; emit({ kind: "reasoning", reasoning: ch }); await delay(12); }
      await delay(200);
      emit({ kind: "tool_dispatch", tool: { id: "t1", name: "glob", args: '{"pattern":"**/*.go"}', readOnly: true } });
      await delay(400);
      emit({ kind: "tool_result", tool: { id: "t1", name: "glob", output: "cmd/tianxuan/main.go\ninternal/agent/agent.go", readOnly: true } });
      await delay(200);
      let reply: string;
      if (isPoetry) reply = "**《山居秋暝》**\n\n> 空山新雨后，天气晚来秋。\n> 明月松间照，清泉石上流。";
      else if (isCodeReq) reply = `好的，"${input}"：项目是 Go，入口在 cmd/tianxuan/main.go。要具体实现什么？`;
      else reply = `收到！**${input}**\n\nGo 项目 tianxuan，核心在 internal/。需要做什么？`;
      for (const ch of reply) { if (cancelled) break; emit({ kind: "text", text: ch }); await delay(10); }
      emit({ kind: "message", text: reply });
      emit({ kind: "usage", usage: { promptTokens: 1200, completionTokens: 200, totalTokens: 1400, cacheHitTokens: 800, cacheMissTokens: 400, sessionCacheHitTokens: 800, sessionCacheMissTokens: 400 } });
      emit({
        kind: "usage",
        usage: {
          promptTokens: 1280,
          completionTokens: 64,
          totalTokens: 1344,
          cacheHitTokens: 1024,
          cacheMissTokens: 256,
          sessionCacheHitTokens: 1024,
          sessionCacheMissTokens: 256,
        },
      });
      emit({ kind: "turn_done" });
    },
    async SubmitDisplay(_display, input) {
      await this.Submit(input);
    },
    async Cancel() {
      cancelled = true;
      emit({ kind: "turn_done" });
    },
    async Approve() {},
    async AnswerQuestion() {},
    async SetPlanMode() {},
    async Compact() {},
    async NewSession() {},
    async Checkpoints() {
      return [];
    },
    async Rewind() {},
    async Fork() {},
    async SummarizeFrom() {},
    async SummarizeUpTo() {},
    async History() {
      return [];
    },
    async ListSessions() {
      return sessions.map((s) => ({ ...s }));
    },
    async ResumeSession(path: string) {
      return [
        { role: "user", content: `(mock) resumed ${path}` },
        { role: "assistant", content: "This is a mock resumed transcript — the real one comes from the kernel." },
      ];
    },
    async DeleteSession(path: string) {
      const i = sessions.findIndex((s) => s.path === path);
      if (i >= 0) sessions.splice(i, 1);
    },
    async RenameSession(path: string, title: string) {
      const s = sessions.find((x) => x.path === path);
      if (s) s.title = title.trim() || undefined;
    },
    async ListWorkspaces() {
      return workspaces.map((path) => ({
        path,
        name: path.split("/").filter(Boolean).pop() ?? path,
        current: path === cwd,
      }));
    },
    async PickWorkspace() {
      // Browser dev has no native dialog; simulate picking a folder and re-root so
      // the topbar folder chip visibly changes.
      return mockSwitchWorkspace(cwd.endsWith("another-project") ? "~/projects/tianxuan" : "~/projects/another-project");
    },
    async SwitchWorkspace(path: string) {
      return mockSwitchWorkspace(path);
    },
    async ContextUsage() {
      return { used: 1280, window: 1_000_000 };
    },
    async TCCAReport() {
      return JSON.stringify({
        l1Size: 12400,
        l2Size: 1200,
        l3Version: 2,
        l4Messages: 18,
        savedByCompact: 82000,
        savedByFork: 100300,
        forkCount: 23,
        savedUsd: 0.24,
        savedLatencyMs: 4500,
        compactionCount: 3,
      });
    },
    async Balance() {
      // Mirror the active mock provider: deepseek-flash carries a balance_url.
      const p = settings.providers.find((x) => x.name === settings.defaultModel);
      if (!p?.balanceUrl) return { available: false, display: "" };
      return { available: true, display: "¥128.50" };
    },
    async Jobs() {
      return []; // browser dev mock has no background jobs
    },
    async Meta(): Promise<Meta> {
      return {
        label: "mock model · browser dev",
        ready: true,
        eventChannel: EVENT_CHANNEL,
        cwd,
        bypass: settings.bypass,
      };
    },
    async Commands() {
      return [
        { name: "new", description: "Start a new session", kind: "builtin" as const },
        { name: "compact", description: "Summarize older history to free up context", kind: "builtin" as const },
        { name: "model", description: "Switch model", kind: "builtin" as const },
        { name: "skill", description: "List skills", kind: "builtin" as const },
        { name: "explore", description: "Investigate the codebase in an isolated subagent", kind: "skill" as const },
        { name: "review", description: "Review the staged diff", hint: "[focus]", kind: "custom" as const },
      ];
    },
    async Capabilities() {
      return { servers: capServers.map((s) => ({ ...s })), skills: capSkills.map((s) => ({ ...s })) };
    },
    async AddMCPServer(input: MCPServerInput) {
      const tools = input.transport === "stdio" ? 3 : 5;
      capServers.push({
        name: input.name,
        transport: input.transport,
        status: "connected",
        tools,
        prompts: 0,
        resources: 0,
        toolList: Array.from({ length: tools }, (_, i) => ({
          name: `${input.name}_tool_${i + 1}`,
          description: `Mock tool ${i + 1} exposed by ${input.name}.`,
        })),
      });
      return tools;
    },
    async RemoveMCPServer(name: string) {
      capServers = capServers.filter((s) => s.name !== name);
    },
    async RetryMCPServer(name: string) {
      capServers = capServers.map((s) =>
        s.name === name ? { ...s, status: "connected", tools: s.tools || 4, error: undefined } : s,
      );
    },
    async SetMCPServerEnabled(name: string, enabled: boolean) {
      capServers = capServers.map((s) =>
        s.name === name
          ? { ...s, status: enabled ? "connected" : "disabled", tools: enabled ? s.tools || 4 : 0, error: undefined }
          : s,
      );
    },
    async SlashArgs(input: string) {
      // Mirror a slice of the real arg hints so the menu is exercisable in browser dev.
      const from = input.lastIndexOf(" ") + 1;
      const cur = input.slice(from);
      const cmd = input.slice(0, input.indexOf(" ") < 0 ? input.length : input.indexOf(" "));
      const subs: Record<string, { label: string; insert: string; hint: string; descend?: boolean }[]> = {
        "/skill": [
          { label: "list", insert: "list", hint: "list skills" },
          { label: "show", insert: "show ", hint: "show a skill's body", descend: true },
          { label: "new", insert: "new ", hint: "scaffold a new skill" },
          { label: "paths", insert: "paths", hint: "show discovery paths" },
        ],
        "/hooks": [
          { label: "list", insert: "list", hint: "list active hooks" },
          { label: "trust", insert: "trust", hint: "trust this project's hooks" },
        ],
        "/model": [
          { label: "deepseek/deepseek-v4-flash", insert: "deepseek/deepseek-v4-flash", hint: "current" },
          { label: "deepseek/deepseek-v4-pro", insert: "deepseek/deepseek-v4-pro", hint: "" },
        ],
      };
      const items = (subs[cmd] ?? [])
        .filter((it) => it.label.toLowerCase().startsWith(cur.toLowerCase()))
        .map((it) => ({ label: it.label, insert: it.insert, hint: it.hint, descend: it.descend ?? false }));
      return { items, from };
    },
    async ListDir(rel: string) {
      // A tiny fake tree so the @ menu is navigable in browser dev.
      if (rel === "" || rel === "./") {
        return [
          { name: "internal", isDir: true },
          { name: "desktop", isDir: true },
          { name: "README.md", isDir: false },
          { name: "go.mod", isDir: false },
        ];
      }
      if (rel === "internal/") {
        return [
          { name: "control", isDir: true },
          { name: "boot", isDir: true },
          { name: "event.go", isDir: false },
        ];
      }
      return [{ name: "file.go", isDir: false }];
    },
    async ReadFile(rel: string) {
      const samples: Record<string, string> = {
        "README.md": "# tianxuan\n\nBrowser-dev workspace preview.\n\n- Chat in the center\n- Browse files on the right\n- Keep sessions on the left\n",
        "go.mod": "module tianxuan\n\ngo 1.23\n",
        "desktop/file.go": "package desktop\n\nfunc main() {\n\tprintln(\"workspace preview\")\n}\n",
        "internal/event.go": "package internal\n\n// mock file used by the browser dev seam\n",
      };
      return {
        path: rel,
        body: samples[rel] ?? `// ${rel}\n\nMock file body from browser dev.`,
        size: samples[rel]?.length ?? 42,
        truncated: false,
        binary: false,
      };
    },
    async OpenWorkspacePath(rel: string) {
      console.info("mock OpenWorkspacePath", rel);
    },
    async RevealWorkspacePath(rel: string) {
      console.info("mock RevealWorkspacePath", rel);
    },
    async SavePastedImage(_dataUrl: string) {
      return ".tianxuan/attachments/mock.png";
    },
    async AttachmentDataURL(_path: string) {
      return "data:image/png;base64,iVBORw0KGgo=";
    },
    async Models() {
      return [
        { ref: "deepseek/deepseek-v4-flash", provider: "deepseek", model: "deepseek-v4-flash", current: true },
        { ref: "deepseek/deepseek-v4-pro", provider: "deepseek", model: "deepseek-v4-pro", current: false },
      ];
    },
    async SetModel() {},
    async Memory() {
      return {
        available: true,
        storeDir: "~/.config/tianxuan/projects/-mock/memory",
        docs: [
          {
            path: "REASONIX.md",
            scope: "project",
            body: "# tianxuan project memory\n\nMock doc shown in the browser dev seam.\n\n## Notes\n\n- prefers concise replies",
          },
          {
            path: "~/.config/tianxuan/REASONIX.md",
            scope: "user",
            body: "# User memory\n\nAlways respond in 中文.",
          },
        ],
        facts: [
          {
            name: "prefers-tabs",
            description: "User prefers tabs",
            type: "user",
            body: "Indent with tabs.",
          },
        ],
        scopes: [
          { scope: "user", path: "~/.config/tianxuan/REASONIX.md" },
          { scope: "project", path: "REASONIX.md" },
          { scope: "local", path: "REASONIX.local.md" },
        ],
      };
    },
    async Remember(scope: string, note: string) {
      emit({ kind: "notice", level: "info", text: `remembered → ${scope}` });
      return `${scope} REASONIX.md (mock): ${note}`;
    },
    async Forget(name: string) {
      emit({ kind: "notice", level: "info", text: `forgot → ${name}` });
    },
    async SaveDoc(path: string, _body: string) {
      emit({ kind: "notice", level: "info", text: `saved → ${path}` });
      return path;
    },
    async Settings() {
      return JSON.parse(JSON.stringify(settings)) as SettingsView;
    },
    async SetDefaultModel(ref: string) {
      settings.defaultModel = ref;
    },
    async SaveProvider(p: ProviderView) {
      const i = settings.providers.findIndex((x) => x.name === p.name);
      if (i >= 0) settings.providers[i] = p;
      else settings.providers.push(p);
    },
    async DeleteProvider(name: string) {
      settings.providers = settings.providers.filter((p) => p.name !== name);
    },
    async SetProviderKey(apiKeyEnv: string) {
      settings.providers.forEach((p) => {
        if (p.apiKeyEnv === apiKeyEnv) p.keySet = true;
      });
    },
    async SetPermissionMode(mode: string) {
      settings.permissions.mode = mode;
    },
    async AddPermissionRule(list: string, rule: string) {
      const k = list as "allow" | "ask" | "deny";
      if (settings.permissions[k] && !settings.permissions[k].includes(rule)) settings.permissions[k].push(rule);
    },
    async RemovePermissionRule(list: string, rule: string) {
      const k = list as "allow" | "ask" | "deny";
      settings.permissions[k] = settings.permissions[k].filter((r) => r !== rule);
    },
    async SetSandbox(bash: string, network: boolean, workspaceRoot: string, allowWrite: string[]) {
      settings.sandbox = { bash, network, workspaceRoot, allowWrite };
    },
    async SetAgentParams(temperature: number, maxSteps: number, systemPrompt: string) {
      settings.agent = { temperature, maxSteps, systemPrompt };
    },
    async SetBypass(on: boolean) {
      settings.bypass = on;
    },
    async Version() {
      return "v1.0.0 (browser dev)";
    },
    async CheckUpdate() {
      // Dev mock advertises an update so the banner and apply flow are exercisable
      // in the browser without a real release behind it.
      return {
        available: true,
        current: "v1.0.0",
        latest: "v1.1.0",
        notes: "- Mock release notes\n- The **Update now** button streams a fake download here.",
        canSelfUpdate: true,
        downloadUrl: "https://github.com/esengine/tianxuan/releases/latest",
        assetSize: 12_345_678,
      };
    },
    async ApplyUpdate() {
      const total = 12_345_678;
      for (let r = 0; r <= total; r += 1_800_000) {
        emitUpdater({ phase: "downloading", received: Math.min(r, total), total });
        await delay(120);
      }
      emitUpdater({ phase: "verifying", received: total, total });
      await delay(500);
      emitUpdater({ phase: "applying", received: total, total });
      await delay(500);
      emitUpdater({ phase: "done", received: total, total });
      // The real shell relaunches here; the mock just stops.
    },
    async OpenDownloadPage() {
      if (typeof window !== "undefined") {
        window.open("https://github.com/esengine/tianxuan/releases/latest", "_blank", "noopener");
      }
    },
  };
}
