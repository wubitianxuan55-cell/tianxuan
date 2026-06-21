// vscode/src/extension.ts — VS Code Extension 入口。
// 启动 tianxuan serve 作为 sidecar 进程，通过 HTTP/SSE 连接。
// 缓存安全: 纯外壳层，不触及 Go 核心（system prompt/tools/messages 全不变）。

import * as vscode from "vscode";
import * as cp from "child_process";
import * as path from "path";

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

  sidecar.stdout?.on("data", (data) => {
    console.log(`[tianxuan] ${data}`);
  });

  sidecar.stderr?.on("data", (data) => {
    console.error(`[tianxuan] ${data}`);
  });

  sidecar.on("exit", (code) => {
    console.log(`[tianxuan] 退出: ${code}`);
    sidecar = null;
  });

  vscode.window.setStatusBarMessage(`$(hubot) tianxuan 已启动: ${sidecarPort}`, 5000);
}

function stopSidecar(): void {
  if (sidecar) {
    sidecar.kill("SIGTERM");
    sidecar = null;
  }
}

function getBinaryPath(context: vscode.ExtensionContext): string | null {
  // 1. 环境变量
  const envPath = process.env.TIANXUAN_BIN;
  if (envPath) return envPath;

  // 2. 预置二进制（扩展打包时放入）
  const bundled = path.join(context.extensionPath, "bin", process.platform === "win32" ? "tianxuan.exe" : "tianxuan");
  try {
    const fs = require("fs");
    if (fs.existsSync(bundled)) return bundled;
  } catch { /* ignore */ }

  // 3. PATH 查找
  return process.platform === "win32" ? "tianxuan.exe" : "tianxuan";
}

// ── Webview 面板 ──

function createWebviewPanel(context: vscode.ExtensionContext): vscode.WebviewPanel {
  const panel = vscode.window.createWebviewPanel(
    "tianxuan.chat",
    "tianxuan",
    vscode.ViewColumn.Beside,
    {
      enableScripts: true,
      retainContextWhenHidden: true,
      localResourceRoots: [vscode.Uri.file(path.join(context.extensionPath, "webview"))],
    }
  );

  // 指向本地 serve 地址（开发阶段），生产环境加载内嵌 webview
  const isDev = process.env.TIANXUAN_DEV === "1";
  if (isDev) {
    panel.webview.html = getDevHtml(sidecarPort);
  } else {
    panel.webview.html = getBundledHtml(context, panel.webview);
  }

  return panel;
}

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
    const fs = require("fs");
    let html = fs.readFileSync(indexPath, "utf-8");
    // 替换资源 URI 为 webview URI
    const baseUri = webview.asWebviewUri(vscode.Uri.file(path.join(context.extensionPath, "webview")));
    html = html.replace(/(href|src)="([^"]+)"/g, (_: string, attr: string, src: string) => {
      if (src.startsWith("http") || src.startsWith("data:")) return `${attr}="${src}"`;
      return `${attr}="${webview.asWebviewUri(vscode.Uri.file(path.join(context.extensionPath, "webview", src)))}"`;
    });
    return html;
  } catch {
    return getDevHtml(8080);
  }
}

// ── 激活入口 ──

export function activate(context: vscode.ExtensionContext) {
  console.log("[tianxuan] VS Code 扩展已激活");

  startSidecar(context);

  // 注册命令：打开面板
  context.subscriptions.push(
    vscode.commands.registerCommand("tianxuan.openPanel", () => {
      createWebviewPanel(context);
    })
  );

  // 注册命令：发送选中内容
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

  // 侧边栏 Webview Provider
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider("tianxuan.panel", {
      resolveWebviewView(webviewView) {
        const isDev = process.env.TIANXUAN_DEV === "1";
        webviewView.webview.options = { enableScripts: true };
        if (isDev) {
          webviewView.webview.html = getDevHtml(sidecarPort);
        } else {
          webviewView.webview.html = getBundledHtml(context, webviewView.webview);
        }
      },
    })
  );
}

export function deactivate() {
  stopSidecar();
}
