// vscode/src/extension.ts — VS Code Extension 入口。
// 启动 tianxuan serve 作为 sidecar 进程，通过 HTTP/SSE 连接。
// Webview 通过 postMessage 调用 VS Code 原生 API（工作区/文件/主题）。
// 缓存安全: 纯外壳层，不触及 Go 核心（system prompt/tools/messages 全不变）。

import * as vscode from "vscode";
import * as cp from "child_process";
import * as path from "path";
import * as fs from "fs";

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
  const envPath = process.env.TIANXUAN_BIN;
  if (envPath) return envPath;
  const bundled = path.join(context.extensionPath, "bin", process.platform === "win32" ? "tianxuan.exe" : "tianxuan");
  if (fs.existsSync(bundled)) return bundled;
  return process.platform === "win32" ? "tianxuan.exe" : "tianxuan";
}

// ── Webview 消息处理 ──
// 处理来自 webview 的 tianxuan:request 消息，调用 VS Code 原生 API。

function setupWebviewHandlers(
  webview: vscode.Webview,
  disposables: vscode.Disposable[]
): void {
  // 发送初始化信息（端口、工作区路径）
  const wsFolders = vscode.workspace.workspaceFolders?.map((f) => ({
    uri: f.uri.fsPath,
    name: f.name,
  })) || [];
  webview.postMessage({
    type: "tianxuan:init",
    port: sidecarPort,
    workspaceFolders: wsFolders,
  });

  // 监听 webview 消息
  disposables.push(
    webview.onDidReceiveMessage(async (msg) => {
      if (!msg || msg.type !== "tianxuan:request") return;

      const { id, method, params } = msg;
      try {
        const result = await handleRequest(method, params || {});
        webview.postMessage({ type: "tianxuan:response", id, result });
      } catch (e: any) {
        webview.postMessage({ type: "tianxuan:response", id, error: e.message || String(e) });
      }
    })
  );

  // 监听 VS Code 工作区变化，通知 webview
  disposables.push(
    vscode.workspace.onDidChangeWorkspaceFolders(() => {
      const folders = vscode.workspace.workspaceFolders?.map((f) => ({
        uri: f.uri.fsPath,
        name: f.name,
      })) || [];
      webview.postMessage({
        type: "tianxuan:workspace-changed",
        workspaceFolders: folders,
      });
    })
  );
}

async function handleRequest(method: string, params: Record<string, unknown>): Promise<unknown> {
  switch (method) {
    case "listWorkspaces": {
      // 返回最近打开的工作区列表（VS Code 不直接暴露，用已打开的代替）
      const folders = vscode.workspace.workspaceFolders?.map((f) => ({
        path: f.uri.fsPath,
        name: f.name,
      })) || [];
      return folders;
    }

    case "pickWorkspace": {
      // 打开文件夹选择器
      const result = await vscode.window.showOpenDialog({
        canSelectFolders: true,
        canSelectFiles: false,
        canSelectMany: false,
        title: "选择工作区文件夹",
      });
      if (result && result.length > 0) {
        const folderPath = result[0].fsPath;
        // 替换当前工作区
        vscode.commands.executeCommand("vscode.openFolder", result[0], false);
        return folderPath;
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
        const filePath = path.isAbsolute(rel) ? rel : path.join(root, rel);
        if (fs.existsSync(filePath)) {
          const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(filePath));
          await vscode.window.showTextDocument(doc, { preview: false });
        }
      }
      break;
    }

    case "revealWorkspacePath": {
      const rel = params.rel as string;
      if (rel) {
        const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "";
        const filePath = path.isAbsolute(rel) ? rel : path.join(root, rel);
        if (fs.existsSync(filePath)) {
          vscode.commands.executeCommand("revealFileInOS", vscode.Uri.file(filePath));
        }
      }
      break;
    }

    case "version": {
      return "8.10.0-vscode";
    }

    default:
      throw new Error(`unknown method: ${method}`);
  }
}

// ── Webview HTML 生成 ──

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
    // 替换资源 URI 为 webview URI
    html = html.replace(/(href|src)="([^"]+)"/g, (_: string, attr: string, src: string) => {
      if (src.startsWith("http") || src.startsWith("data:")) return `${attr}="${src}"`;
      return `${attr}="${webview.asWebviewUri(vscode.Uri.file(path.join(context.extensionPath, "webview", src)))}"`;
    });
    // 注入 CSP，允许 webview 通过 HTTP/SSE 连接 tianxuan serve
    const csp = [
      `default-src 'none'`,
      `script-src ${webview.cspSource} 'unsafe-inline'`,
      `style-src ${webview.cspSource} 'unsafe-inline'`,
      `connect-src http://127.0.0.1:${sidecarPort} http://localhost:${sidecarPort}`,
      `img-src ${webview.cspSource} https: data:`,
      `font-src ${webview.cspSource}`,
    ].join("; ");
    html = html.replace("<head>", `<head><meta http-equiv="Content-Security-Policy" content="${csp}">`);
    return html;
  } catch {
    return getDevHtml(8080);
  }
}

// ── Webview 创建 ──

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

  // VS Code webview 不支持直接访问 localhost HTTP，需要 CSP 放宽
  // 或者用 serve webui 内嵌模式。这里用宽松 CSP 允许 connect-src。
  panel.webview.options = {
    enableScripts: true,
    localResourceRoots: [vscode.Uri.file(path.join(context.extensionPath, "webview"))],
  };

  const isDev = process.env.TIANXUAN_DEV === "1";
  panel.webview.html = isDev
    ? getDevHtml(sidecarPort)
    : getBundledHtml(context, panel.webview);

  // 设置消息处理
  setupWebviewHandlers(panel.webview, [panel]);

  return panel;
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
        setupWebviewHandlers(webviewView.webview, []);
      },
    })
  );
}

export function deactivate() {
  stopSidecar();
}
