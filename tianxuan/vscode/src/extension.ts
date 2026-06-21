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
      const rel = params.rel as string;
      if (rel) {
        const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "";
        const fp = path.isAbsolute(rel) ? rel : path.join(root, rel);
        if (fs.existsSync(fp)) {
          const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(fp));
          await vscode.window.showTextDocument(doc, { preview: false });
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

    default:
      throw new Error(`unknown method: ${method}`);
  }
}

// ── Webview 消息处理 ──────────────────────────────────────────────────

function setupWebviewHandlers(
  webview: vscode.Webview,
  disposables: vscode.Disposable[]
): void {
  // 发送初始化
  const wsFolders = (vscode.workspace.workspaceFolders || []).map((f) => ({
    uri: f.uri.fsPath, name: f.name,
  }));
  webview.postMessage({ type: "tianxuan:init", port: sidecarPort, workspaceFolders: wsFolders });

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
          panel.webview.postMessage({ type: "submit", text: selection });
        }
      }
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
}

export function deactivate() {
  stopSidecar();
}
