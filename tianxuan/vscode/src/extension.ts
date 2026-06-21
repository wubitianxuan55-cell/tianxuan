// vscode/src/extension.ts — VS Code Extension 入口。
// 启动 tianxuan serve 作为 sidecar 进程，通过 HTTP/SSE 连接。
// Webview 的所有 HTTP/SSE 请求经 postMessage 代理，由扩展主进程转发到 serve。
// 缓存安全: 纯外壳层，不触及 Go 核心（system prompt/tools/messages 全不变）。

import * as vscode from "vscode";
import * as cp from "child_process";
import * as path from "path";
import * as fs from "fs";
import * as http from "http";

let sidecar: cp.ChildProcess | null = null;
let sidecarPort = 8080;
let statusBar: vscode.StatusBarItem;

// ── 状态栏 ────────────────────────────────────────────────────────────

function createStatusBar(context: vscode.ExtensionContext): void {
  statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
  statusBar.command = "tianxuan.openPanel";
  statusBar.text = "$(hubot) tianxuan";
  statusBar.tooltip = "打开 tianxuan AI 面板";
  statusBar.show();
  context.subscriptions.push(statusBar);
}

function setStatusRunning(running: boolean): void {
  if (!statusBar) return;
  if (running) {
    statusBar.text = "$(loading~spin) tianxuan";
    statusBar.backgroundColor = undefined;
  } else {
    statusBar.text = "$(hubot) tianxuan";
    statusBar.backgroundColor = undefined;
  }
}

// ── 辅助函数 ──────────────────────────────────────────────────────────

// 获取光标下的函数/方法签名（通过 DocumentSymbolProvider）
async function getSymbolAtCursor(
  document: vscode.TextDocument,
  position: vscode.Position,
): Promise<{ name: string; kind: string; range: vscode.Range } | null> {
  const symbols = await vscode.commands.executeCommand<vscode.DocumentSymbol[]>(
    "vscode.executeDocumentSymbolProvider",
    document.uri,
  );
  if (!symbols || symbols.length === 0) return null;
  function find(ss: vscode.DocumentSymbol[]): vscode.DocumentSymbol | null {
    for (const s of ss) {
      if (s.range.contains(position)) return s;
      const child = find(s.children);
      if (child) return child;
    }
    return null;
  }
  const sym = find(symbols);
  if (!sym) return null;
  return { name: sym.name, kind: vscode.SymbolKind[sym.kind], range: sym.range };
}

// 根据语言 ID 推断测试框架
function getTestFramework(languageId: string): string {
  switch (languageId) {
    case "go": return "testing";
    case "typescript": case "typescriptreact": return "vitest";
    case "javascript": case "javascriptreact": return "jest";
    case "python": return "pytest";
    case "rust": return "#[test]";
    case "java": return "JUnit 5";
    case "csharp": return "xUnit";
    case "cpp": case "c": return "Google Test";
    default: return "the standard test framework";
  }
}

// 判断字符是否语义边界（补全触发点）
function isCompletionBoundary(char: string): boolean {
  return char === "." || char === "(" || char === " " || char === "\n" || char === ":" || char === ">";
}

// diff 预览辅助（供命令使用，不走 webview postMessage）
async function vscodeProxyForCmds(original: string, modified: string, title: string): Promise<void> {
  const os = require("os");
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "tianxuan-diff-"));
  const origPath = path.join(tmpDir, "original.diff");
  const modPath = path.join(tmpDir, "modified.diff");
  fs.writeFileSync(origPath, original, "utf-8");
  fs.writeFileSync(modPath, modified, "utf-8");
  await vscode.commands.executeCommand("vscode.diff",
    vscode.Uri.file(origPath), vscode.Uri.file(modPath), title);
  setTimeout(() => {
    try { fs.rmSync(tmpDir, { recursive: true, force: true }); } catch { /* */ }
  }, 5000);
}

// ── 主题同步 ──────────────────────────────────────────────────────────

function getVSThemeKind(): "dark" | "light" {
  switch (vscode.window.activeColorTheme.kind) {
    case vscode.ColorThemeKind.Light:
    case vscode.ColorThemeKind.HighContrastLight:
      return "light";
    default:
      return "dark";
  }
}

function sendTheme(webview: vscode.Webview): void {
  webview.postMessage({ type: "tianxuan:theme-changed", theme: getVSThemeKind() });
}

// ── Sidecar 生命周期 ──

function startSidecar(context: vscode.ExtensionContext): void {
  const bin = getBinaryPath(context);
  if (!bin) {
    vscode.window.showErrorMessage("tianxuan: 找不到二进制文件，请确认已安装 tianxuan CLI");
    return;
  }
  const cwd = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || process.cwd();
  sidecar = cp.spawn(bin, ["serve", "--port", String(sidecarPort)], {
    cwd,
    stdio: ["pipe", "pipe", "pipe"],
  });
  sidecar.stdout?.on("data", (d: Buffer) => console.log(`[tianxuan] ${d}`));
  sidecar.stderr?.on("data", (d: Buffer) => console.error(`[tianxuan] ${d}`));
  sidecar.on("exit", (code: number | null) => {
    console.log(`[tianxuan] 退出: ${code}`);
    sidecar = null;
  });
  vscode.window.setStatusBarMessage(`$(hubot) tianxuan 已启动: ${sidecarPort}`, 5000);
}

function stopSidecar(): void {
  if (sidecar) { sidecar.kill("SIGTERM"); sidecar = null; }
}

function getBinaryPath(context: vscode.ExtensionContext): string | null {
  const envPath = process.env.TIANXUAN_BIN;
  if (envPath) return envPath;
  const bundled = path.join(context.extensionPath, "bin", process.platform === "win32" ? "tianxuan.exe" : "tianxuan");
  if (fs.existsSync(bundled)) return bundled;
  return process.platform === "win32" ? "tianxuan.exe" : "tianxuan";
}

// ── HTTP 代理 ─────────────────────────────────────────────────────────
// 向 tianxuan serve 发出 HTTP 请求，返回 { status, body }。
// webview 因 CSP 限制不能直接 fetch localhost，所有调用经此代理。

function serveURL(p: string): string {
  return `http://127.0.0.1:${sidecarPort}${p}`;
}

async function proxyFetch(method: string, path: string, body?: unknown): Promise<{ status: number; body: string }> {
  const url = serveURL(path);
  const opts: http.RequestOptions = {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : {},
  };
  return new Promise((resolve, reject) => {
    const req = http.request(url, opts, (res) => {
      let data = "";
      res.on("data", (chunk: Buffer) => { data += chunk.toString(); });
      res.on("end", () => resolve({ status: res.statusCode || 500, body: data }));
    });
    req.on("error", (err: Error) => reject(err));
    if (body !== undefined) req.write(JSON.stringify(body));
    req.end();
  });
}

// ── SSE 代理 ──────────────────────────────────────────────────────────
// 连接到 tianxuan serve 的 /events SSE 流，逐行转发给 webview。
// 每个 webview 只有一个活跃的 SSE 连接；面板关闭时自动断开。

const sseStreams = new Map<vscode.Webview, http.ClientRequest>();

function connectSSE(webview: vscode.Webview): void {
  // 避免重复连接
  if (sseStreams.has(webview)) return;

  const url = serveURL("/events");
  http.get(url, (res) => {
    let buf = "";
    res.on("data", (chunk: Buffer) => {
      buf += chunk.toString();
      // SSE 协议：以 \n\n 分隔事件帧
      while (true) {
        const idx = buf.indexOf("\n\n");
        if (idx === -1) break;
        const frame = buf.slice(0, idx).trim();
        buf = buf.slice(idx + 2);
        // 提取 data: 行
        for (const line of frame.split("\n")) {
          if (line.startsWith("data: ")) {
            const data = line.slice(6);
            webview.postMessage({ type: "tianxuan:sse:event", data });
          }
        }
      }
    });
    res.on("end", () => { sseStreams.delete(webview); });
    res.on("error", () => { sseStreams.delete(webview); });
  }).on("error", () => { sseStreams.delete(webview); });
}

function closeSSE(webview: vscode.Webview): void {
  const req = sseStreams.get(webview);
  if (req) { req.destroy(); sseStreams.delete(webview); }
}

// ── 请求分发 ──────────────────────────────────────────────────────────

async function handleRequest(
  method: string,
  params: Record<string, unknown>,
  webview: vscode.Webview
): Promise<unknown> {
  switch (method) {
    // ── HTTP 代理 ──
    case "fetch": {
      const { method: m, path: p, body } = params as {
        method: string; path: string; body?: unknown;
      };
      return await proxyFetch(m, p, body);
    }

    // ── SSE ──
    case "sse:connect":
      connectSSE(webview);
      return null;

    case "sse:close":
      closeSSE(webview);
      return null;

    // ── 工作区 API ──
    case "listWorkspaces": {
      return (vscode.workspace.workspaceFolders || []).map((f) => ({
        path: f.uri.fsPath, name: f.name,
      }));
    }

    case "pickWorkspace": {
      const result = await vscode.window.showOpenDialog({
        canSelectFolders: true, canSelectFiles: false,
        canSelectMany: false, title: "选择工作区文件夹",
      });
      if (result && result.length > 0) {
        vscode.commands.executeCommand("vscode.openFolder", result[0], false);
        return result[0].fsPath;
      }
      return "";
    }

    case "switchWorkspace": {
      const p = params.path as string;
      if (p && fs.existsSync(p)) {
        vscode.commands.executeCommand("vscode.openFolder", vscode.Uri.file(p), false);
        return p;
      }
      return "";
    }

    case "openWorkspacePath": {
      // 解析文件路径，支持 "file.go:42" 和 "file.go:42:10" 格式
      let rel = params.rel as string;
      let lineNum = 0;
      let colNum = 0;
      if (rel) {
        // 检测行号模式：path:line[:col]
        const match = rel.match(/^(.+?):(\d+)(?::(\d+))?$/);
        if (match) {
          rel = match[1];
          lineNum = parseInt(match[2], 10);
          colNum = match[3] ? parseInt(match[3], 10) : 0;
        }
        const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "";
        const fp = path.isAbsolute(rel) ? rel : path.join(root, rel);
        if (fs.existsSync(fp)) {
          const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(fp));
          const pos = new vscode.Position(
            Math.max(0, lineNum - 1), // 0-based
            Math.max(0, colNum > 0 ? colNum - 1 : 0),
          );
          await vscode.window.showTextDocument(doc, {
            preview: false,
            selection: new vscode.Range(pos, pos),
          });
        }
      }
      break;
    }

    case "revealWorkspacePath": {
      const rel = params.rel as string;
      if (rel) {
        const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "";
        const fp = path.isAbsolute(rel) ? rel : path.join(root, rel);
        if (fs.existsSync(fp)) {
          vscode.commands.executeCommand("revealFileInOS", vscode.Uri.file(fp));
        }
      }
      break;
    }

    case "version":
      return "8.12.0-vscode";

    // ── 编辑器 API ──
    case "getDiagnostics": {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return [];
      const diags = vscode.languages.getDiagnostics(editor.document.uri);
      return diags.map((d) => ({
        message: d.message,
        severity: d.severity === vscode.DiagnosticSeverity.Error ? "error"
          : d.severity === vscode.DiagnosticSeverity.Warning ? "warning"
          : d.severity === vscode.DiagnosticSeverity.Information ? "info"
          : "hint",
        range: {
          start: { line: d.range.start.line, character: d.range.start.character },
          end: { line: d.range.end.line, character: d.range.end.character },
        },
        source: d.source,
      }));
    }

    case "getEditorContext": {
      const ed = vscode.window.activeTextEditor;
      if (!ed) return { file: "", language: "", selection: "", cursorLine: 0 };
      const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "";
      const relPath = ed.document.uri.fsPath.startsWith(root)
        ? path.relative(root, ed.document.uri.fsPath)
        : ed.document.uri.fsPath;
      return {
        file: relPath,
        language: ed.document.languageId,
        selection: ed.document.getText(ed.selection) || "",
        cursorLine: ed.selection.active.line,
      };
    }

    case "applyEdit": {
      const { filePath, edits } = params as {
        filePath: string; edits: { range: { start: { line: number; character: number }; end: { line: number; character: number } }; newText: string }[];
      };
      const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "";
      const fp = path.isAbsolute(filePath) ? filePath : path.join(root, filePath);
      const uri = vscode.Uri.file(fp);
      const we = new vscode.WorkspaceEdit();
      for (const e of edits) {
        we.replace(uri, new vscode.Range(
          new vscode.Position(e.range.start.line, e.range.start.character),
          new vscode.Position(e.range.end.line, e.range.end.character),
        ), e.newText);
      }
      const ok = await vscode.workspace.applyEdit(we);
      return { applied: ok };
    }

    case "diffPreview": {
      const { original, modified, title } = params as {
        original: string; modified: string; title?: string;
      };
      // 写入临时文件，然后调用 VS Code diff 编辑器
      const os = require("os");
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "tianxuan-diff-"));
      const origPath = path.join(tmpDir, "original.diff");
      const modPath = path.join(tmpDir, "modified.diff");
      fs.writeFileSync(origPath, original, "utf-8");
      fs.writeFileSync(modPath, modified, "utf-8");
      await vscode.commands.executeCommand("vscode.diff",
        vscode.Uri.file(origPath), vscode.Uri.file(modPath),
        title || "tianxuan Diff");
      // 延迟清理临时文件（diff 视图已捕获内容快照）
      setTimeout(() => {
        try { fs.rmSync(tmpDir, { recursive: true, force: true }); } catch { /* */ }
      }, 5000);
      return null;
    }

    default:
      throw new Error(`unknown method: ${method}`);
  }
}

// ── Webview 消息处理 ──────────────────────────────────────────────────

function setupWebviewHandlers(
  webview: vscode.Webview,
  disposables: vscode.Disposable[]
): void {
  // 发送初始化（含主题信息）
  const wsFolders = (vscode.workspace.workspaceFolders || []).map((f) => ({
    uri: f.uri.fsPath, name: f.name,
  }));
  webview.postMessage({
    type: "tianxuan:init",
    port: sidecarPort,
    workspaceFolders: wsFolders,
    theme: getVSThemeKind(),
  });

  // 监听 webview 请求
  disposables.push(
    webview.onDidReceiveMessage(async (msg) => {
      if (!msg || msg.type !== "tianxuan:request") return;
      const { id, method, params } = msg;
      try {
        const result = await handleRequest(method, params || {}, webview);
        webview.postMessage({ type: "tianxuan:response", id, result });
      } catch (e: any) {
        webview.postMessage({ type: "tianxuan:response", id, error: e.message || String(e) });
      }
    })
  );

  // 面板关闭时断开 SSE
  disposables.push({ dispose: () => closeSSE(webview) });

  // 主题变化通知
  disposables.push(
    vscode.window.onDidChangeActiveColorTheme(() => sendTheme(webview))
  );

  // 工作区变化通知
  disposables.push(
    vscode.workspace.onDidChangeWorkspaceFolders(() => {
      webview.postMessage({
        type: "tianxuan:workspace-changed",
        workspaceFolders: (vscode.workspace.workspaceFolders || []).map((f) => ({
          uri: f.uri.fsPath, name: f.name,
        })),
      });
    })
  );
}

// ── HTML 生成 ─────────────────────────────────────────────────────────

function getDevHtml(port: number): string {
  return `<!DOCTYPE html>
<html><head><meta charset="UTF-8"><title>tianxuan</title></head>
<body><p>开发模式：请启动 web dev server（cd web && npm run dev）</p>
<p>然后访问 <a href="http://localhost:5174">localhost:5174</a></p>
</body></html>`;
}

function getBundledHtml(context: vscode.ExtensionContext, webview: vscode.Webview): string {
  const indexPath = path.join(context.extensionPath, "webview", "index.html");
  try {
    let html = fs.readFileSync(indexPath, "utf-8");
    html = html.replace(/(href|src)="([^"]+)"/g, (_: string, attr: string, src: string) => {
      if (src.startsWith("http") || src.startsWith("data:")) return `${attr}="${src}"`;
      return `${attr}="${webview.asWebviewUri(vscode.Uri.file(path.join(context.extensionPath, "webview", src)))}"`;
    });
    // CSP: 只允许 vscode-webview 资源，禁止 connect-src（全部走 postMessage 代理）
    const csp = [
      `default-src 'none'`,
      `script-src ${webview.cspSource} 'unsafe-inline'`,
      `style-src ${webview.cspSource} 'unsafe-inline'`,
      `img-src ${webview.cspSource} https: data:`,
      `font-src ${webview.cspSource}`,
    ].join("; ");
    html = html.replace("<head>", `<head><meta http-equiv="Content-Security-Policy" content="${csp}">`);
    return html;
  } catch {
    return getDevHtml(8080);
  }
}

// ── Webview 创建 ──────────────────────────────────────────────────────

function createWebviewPanel(context: vscode.ExtensionContext): vscode.WebviewPanel {
  const panel = vscode.window.createWebviewPanel(
    "tianxuan.chat", "tianxuan", vscode.ViewColumn.Beside,
    {
      enableScripts: true,
      retainContextWhenHidden: true,
      localResourceRoots: [vscode.Uri.file(path.join(context.extensionPath, "webview"))],
    }
  );
  panel.webview.options = {
    enableScripts: true,
    localResourceRoots: [vscode.Uri.file(path.join(context.extensionPath, "webview"))],
  };
  panel.webview.html = process.env.TIANXUAN_DEV === "1"
    ? getDevHtml(sidecarPort)
    : getBundledHtml(context, panel.webview);
  setupWebviewHandlers(panel.webview, [panel]);
  return panel;
}

// ── 激活入口 ──────────────────────────────────────────────────────────

export function activate(context: vscode.ExtensionContext) {
  console.log("[tianxuan] VS Code 扩展已激活");
  startSidecar(context);

  // 状态栏
  createStatusBar(context);

  // 主题变更 → 广播到所有已打开的 webview（sidebar view 可能尚未创建，ok）
  context.subscriptions.push(
    vscode.window.onDidChangeActiveColorTheme(() => {
      // 侧边栏 view 的 webview 由 setupWebviewHandlers 自行订阅
      // 此处的独立 panel 也由各自的 setupWebviewHandlers 处理
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.openPanel", () => {
      createWebviewPanel(context);
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.submitSelection", () => {
      const editor = vscode.window.activeTextEditor;
      if (editor) {
        const selection = editor.document.getText(editor.selection);
        if (selection) {
          const panel = createWebviewPanel(context);
          // 通过桥接通道发送选中文本，走 web bridge → app.Submit 路径
          panel.webview.postMessage({ type: "tianxuan:submit-text", text: selection });
        }
      }
    })
  );

  // 右键菜单：用 tianxuan 解释选中代码
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.explainSelection", () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const selection = editor.document.getText(editor.selection);
      if (!selection) return;
      const fileLang = editor.document.languageId;
      const prompt = `请解释以下 ${fileLang} 代码：\n\`\`\`${fileLang}\n${selection}\n\`\`\``;
      const panel = createWebviewPanel(context);
      panel.webview.postMessage({ type: "tianxuan:submit-text", text: prompt });
    })
  );

  // 右键菜单：用 tianxuan 审查选中代码
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.reviewSelection", () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const selection = editor.document.getText(editor.selection);
      const target = selection || editor.document.getText();
      const fileLang = editor.document.languageId;
      const prompt = `请审查以下 ${fileLang} 代码，指出潜在问题：\n\`\`\`${fileLang}\n${target}\n\`\`\``;
      const panel = createWebviewPanel(context);
      panel.webview.postMessage({ type: "tianxuan:submit-text", text: prompt });
    })
  );

  // 右键菜单：用 tianxuan 修复选中代码
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.fixSelection", () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const selection = editor.document.getText(editor.selection);
      const target = selection || editor.document.getText();
      const fileLang = editor.document.languageId;
      const prompt = `请修复以下 ${fileLang} 代码中的问题：\n\`\`\`${fileLang}\n${target}\n\`\`\`\n\n如有修复建议，请给出完整的修改后代码。`;
      const panel = createWebviewPanel(context);
      panel.webview.postMessage({ type: "tianxuan:submit-text", text: prompt });
    })
  );

  // ── 生成文档注释 ───────────────────────────────────────────────────
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.generateDocstring", async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const pos = editor.selection.active;
      const sym = await getSymbolAtCursor(editor.document, pos);
      if (!sym) {
        vscode.window.showInformationMessage("tianxuan: 请将光标放在函数/类上");
        return;
      }
      const funcText = editor.document.getText(sym.range);
      const lid = editor.document.languageId;
      const prompt = `Generate documentation comment for this ${lid} code. Use ${lid}-appropriate doc syntax. Output ONLY the comment — no code fences.\nCode:\n\`\`\`${lid}\n${funcText}\n\`\`\``;
      try {
        const resp = await proxyFetch("POST", "/complete", { context: prompt, language: lid, file: editor.document.fileName });
        if (resp.status >= 400 || !resp.body) return;
        const data = JSON.parse(resp.body) as { text?: string };
        if (!data.text) return;
        const insertPos = new vscode.Position(sym.range.start.line, 0);
        const indent = editor.document.lineAt(sym.range.start.line).text.match(/^(\s*)/)?.[1] || "";
        const indented = data.text.split("\n").map((l: string) => indent + l).join("\n") + "\n";
        await editor.edit((eb) => eb.insert(insertPos, indented));
      } catch { /* ignore */ }
    })
  );

  // ── 生成单元测试 ───────────────────────────────────────────────────
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.generateTest", async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const pos = editor.selection.active;
      const sym = await getSymbolAtCursor(editor.document, pos);
      if (!sym) {
        vscode.window.showInformationMessage("tianxuan: 请将光标放在函数/类上");
        return;
      }
      const funcText = editor.document.getText(sym.range);
      const lid = editor.document.languageId;
      const framework = getTestFramework(lid);
      const prompt = `Write unit test(s) for this function using ${framework}. Include edge cases. Output ONLY test code — no code fences.\nFunction:\n\`\`\`${lid}\n${funcText}\n\`\`\``;
      try {
        const resp = await proxyFetch("POST", "/complete", { context: prompt, language: lid, file: editor.document.fileName });
        if (resp.status >= 400 || !resp.body) return;
        const data = JSON.parse(resp.body) as { text?: string };
        if (!data.text) return;
        await vscodeProxyForCmds("", data.text, sym.name + " — Unit Test (" + framework + ")");
      } catch { /* ignore */ }
    })
  );

  // ── 终端错误解释 ───────────────────────────────────────────────────
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.explainTerminal", async () => {
      const text = await vscode.window.showInputBox({
        prompt: "粘贴终端错误信息（选中终端文本 Ctrl+C 后粘贴）",
        placeHolder: "粘贴错误输出...",
      });
      if (!text) return;
      const prompt = "请用中文解释以下错误并给出修复建议：\n\`\`\`\n" + text + "\n\`\`\`";
      const panel = createWebviewPanel(context);
      panel.webview.postMessage({ type: "tianxuan:submit-text", text: prompt });
    })
  );

  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider("tianxuan.panel", {
      resolveWebviewView(webviewView) {
        webviewView.webview.options = { enableScripts: true };
        webviewView.webview.html = process.env.TIANXUAN_DEV === "1"
          ? getDevHtml(sidecarPort)
          : getBundledHtml(context, webviewView.webview);
        setupWebviewHandlers(webviewView.webview, []);
      },
    })
  );

  // ── Inline 代码补全 ─────────────────────────────────────────────────
  // 注册补全提供器：用户输入在语义边界处暂停 300ms 后触发。

  const completionThrottle = 2000; // 两次补全之间至少隔 2 秒
  let lastCompletionTs = 0;
  let completionDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  context.subscriptions.push(
    vscode.languages.registerInlineCompletionItemProvider(
      { pattern: "**" },
      {
        async provideInlineCompletionItems(
          document: vscode.TextDocument,
          position: vscode.Position,
          _context: vscode.InlineCompletionContext,
        ): Promise<vscode.InlineCompletionList> {
          // 节流检查
          const now = Date.now();
          if (now - lastCompletionTs < completionThrottle) {
            return new vscode.InlineCompletionList([]);
          }
          // 语义边界检查：只在特定字符后触发补全
          if (position.character > 0) {
            const charBefore = document.lineAt(position.line).text[position.character - 1];
            if (!isCompletionBoundary(charBefore)) {
              return new vscode.InlineCompletionList([]);
            }
          }

          // 防抖：延迟 300ms，期间有新输入则取消
          return new Promise((resolve) => {
            if (completionDebounceTimer) clearTimeout(completionDebounceTimer);
            completionDebounceTimer = setTimeout(async () => {
              lastCompletionTs = Date.now();
              const startLine = Math.max(0, position.line - 50);
              const endLine = Math.min(document.lineCount - 1, position.line + 20);
              const beforeRange = new vscode.Range(startLine, 0, position.line, position.character);
              const afterRange = new vscode.Range(position.line, position.character, endLine, document.lineAt(endLine).text.length);
              const ctxText = document.getText(beforeRange) + "〈CURSOR〉" + document.getText(afterRange);
              try {
                const resp = await proxyFetch("POST", "/complete", { context: ctxText, language: document.languageId, file: document.fileName });
                if (resp.status >= 400) { resolve(new vscode.InlineCompletionList([])); return; }
                const data = JSON.parse(resp.body) as { text?: string };
                if (!data.text || data.text.trim().length === 0) {
                  resolve(new vscode.InlineCompletionList([])); return;
                }
                resolve(new vscode.InlineCompletionList([new vscode.InlineCompletionItem(data.text)]));
              } catch {
                resolve(new vscode.InlineCompletionList([]));
              }
            }, 300);
          });
        },
      },
    )
  );

  // ── Hover 提示 ──────────────────────────────────────────────────────

  context.subscriptions.push(
    vscode.languages.registerHoverProvider(
      { pattern: "**" },
      {
        async provideHover(
          document: vscode.TextDocument,
          position: vscode.Position,
        ): Promise<vscode.Hover | null> {
          // 获取光标下的单词
          const wordRange = document.getWordRangeAtPosition(position);
          if (!wordRange) return null;
          const word = document.getText(wordRange);
          if (word.length < 2) return null;

          try {
            const resp = await proxyFetch("POST", "/complete", {
              context: `Explain what "${word}" is in the context of:\n\`\`\`${document.languageId}\n${document.getText(new vscode.Range(
                Math.max(0, position.line - 5), 0,
                Math.min(document.lineCount - 1, position.line + 5),
                document.lineAt(Math.min(document.lineCount - 1, position.line + 5)).text.length,
              ))}\n\`\`\`\n\nBriefly explain "${word}" in one sentence.`,
              language: "text",
              file: document.fileName,
            });
            if (resp.status >= 400) return null;
            const data = JSON.parse(resp.body) as { text?: string };
            if (!data.text) return null;
            return new vscode.Hover(new vscode.MarkdownString(data.text));
          } catch {
            return null;
          }
        },
      },
    )
  );

  // ── Quick Fix 代码动作 (CodeActionProvider) ────────────────────────
  context.subscriptions.push(
    vscode.languages.registerCodeActionsProvider(
      { pattern: "**" },
      {
        async provideCodeActions(
          _document: vscode.TextDocument,
          _range: vscode.Range | vscode.Selection,
          ctx: vscode.CodeActionContext,
        ): Promise<vscode.CodeAction[]> {
          const diags = ctx.diagnostics.filter(
            (d) => d.severity === vscode.DiagnosticSeverity.Error || d.severity === vscode.DiagnosticSeverity.Warning,
          );
          if (diags.length === 0) return [];
          const d = diags[0];
          const action = new vscode.CodeAction(
            "$(hubot) tianxuan 修复: " + d.message.slice(0, 60),
            vscode.CodeActionKind.QuickFix,
          );
          action.diagnostics = [d];
          action.command = { command: "tianxuan.quickFix", title: "tianxuan 修复", arguments: [_document, d] };
          return [action];
        },
      },
    )
  );

  // Quick Fix 执行命令
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.quickFix", async (document: vscode.TextDocument, d: vscode.Diagnostic) => {
      const startLine = Math.max(0, d.range.start.line - 15);
      const endLine = Math.min(document.lineCount - 1, d.range.end.line + 15);
      const code = document.getText(new vscode.Range(startLine, 0, endLine, document.lineAt(endLine).text.length));
      const prompt = `Fix this ${document.languageId} code issue: "${d.message}". Output only corrected code — no markdown, no explanation.\nCode:\n\`\`\`${document.languageId}\n${code}\n\`\`\``;
      try {
        const resp = await proxyFetch("POST", "/complete", { context: prompt, language: document.languageId, file: document.fileName });
        if (resp.status >= 400 || !resp.body) return;
        const data = JSON.parse(resp.body) as { text?: string };
        if (!data.text) return;
        await vscodeProxyForCmds(code, data.text, "tianxuan 修复: " + d.message.slice(0, 60));
      } catch { /* ignore */ }
    })
  );

  // ── CodeLens ────────────────────────────────────────────────────────
  context.subscriptions.push(
    vscode.languages.registerCodeLensProvider(
      { pattern: "**" },
      {
        async provideCodeLenses(document: vscode.TextDocument): Promise<vscode.CodeLens[]> {
          const symbols = await vscode.commands.executeCommand<vscode.DocumentSymbol[]>(
            "vscode.executeDocumentSymbolProvider", document.uri,
          );
          if (!symbols || symbols.length === 0) return [];
          const lenses: vscode.CodeLens[] = [];
          function collect(ss: vscode.DocumentSymbol[]) {
            for (const s of ss) {
              if (s.kind === vscode.SymbolKind.Function || s.kind === vscode.SymbolKind.Method) {
                const pos = new vscode.Position(s.range.start.line, s.range.start.character);
                lenses.push(new vscode.CodeLens(new vscode.Range(pos, pos), {
                  title: "$(hubot) 解释", command: "tianxuan.explainFunction", arguments: [document, s],
                }));
                lenses.push(new vscode.CodeLens(new vscode.Range(pos, pos), {
                  title: "$(beaker) 测试", command: "tianxuan.generateTestForSymbol", arguments: [document, s],
                }));
                lenses.push(new vscode.CodeLens(new vscode.Range(pos, pos), {
                  title: "$(book) 文档", command: "tianxuan.generateDocForSymbol", arguments: [document, s],
                }));
              }
              collect(s.children);
            }
          }
          collect(symbols);
          return lenses;
        },
      },
    )
  );

  // CodeLens: 解释函数
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.explainFunction", (document: vscode.TextDocument, sym: vscode.DocumentSymbol) => {
      const funcText = document.getText(sym.range);
      const prompt = "请解释以下 " + document.languageId + " 函数：\n\`\`\`" + document.languageId + "\n" + funcText + "\n\`\`\`";
      const panel = createWebviewPanel(context);
      panel.webview.postMessage({ type: "tianxuan:submit-text", text: prompt });
    })
  );

  // CodeLens: 生成测试
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.generateTestForSymbol", async (document: vscode.TextDocument, sym: vscode.DocumentSymbol) => {
      const funcText = document.getText(sym.range);
      const lid = document.languageId;
      const framework = getTestFramework(lid);
      const prompt = `Write unit test(s) for this function using ${framework}. Include edge cases. Output ONLY test code — no code fences.\nFunction:\n\`\`\`${lid}\n${funcText}\n\`\`\``;
      try {
        const resp = await proxyFetch("POST", "/complete", { context: prompt, language: lid, file: document.fileName });
        if (resp.status >= 400 || !resp.body) return;
        const data = JSON.parse(resp.body) as { text?: string };
        if (!data.text) return;
        await vscodeProxyForCmds("", data.text, sym.name + " — Unit Test (" + framework + ")");
      } catch { /* ignore */ }
    })
  );

  // CodeLens: 生成文档
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.generateDocForSymbol", async (document: vscode.TextDocument, sym: vscode.DocumentSymbol) => {
      const funcText = document.getText(sym.range);
      const lid = document.languageId;
      const prompt = `Generate documentation comment for this ${lid} code. Use ${lid}-appropriate doc syntax. Output ONLY the comment — no code fences.\nCode:\n\`\`\`${lid}\n${funcText}\n\`\`\``;
      try {
        const resp = await proxyFetch("POST", "/complete", { context: prompt, language: lid, file: document.fileName });
        if (resp.status >= 400 || !resp.body) return;
        const data = JSON.parse(resp.body) as { text?: string };
        if (!data.text) return;
        const insertPos = new vscode.Position(sym.range.start.line, 0);
        const editor = await vscode.window.showTextDocument(document, { preview: false });
        const indent = document.lineAt(sym.range.start.line).text.match(/^(\s*)/)?.[1] || "";
        const indented = data.text.split("\n").map((l: string) => indent + l).join("\n") + "\n";
        await editor.edit((eb) => eb.insert(insertPos, indented));
      } catch { /* ignore */ }
    })
  );
}

export function deactivate() {
  stopSidecar();
}
