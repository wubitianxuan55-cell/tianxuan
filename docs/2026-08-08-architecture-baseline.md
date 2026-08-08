# 架构调整前基线（V10.165.0）

> 记录于 2026-08-08。大架构调整开始前的稳定基线：功能、版本、tag、回滚方法。
> 调整过程中如需回滚，以本文件为准。

## 稳定基线

- **版本**: V10.165.0（桌面安装版，Windows）
- **发布提交**: `d00d17e`（V10.165.0 发布: AI 自主截图工具 capture_screen）
- **发布分支**: `release/v10.110.0`（远端同步，工作区干净）
- **发布 tag**: `desktop-v10.165.0` → `d00d17e`（远端已修正并验证）
- **安装包**: `https://github.com/wubitianxuan55-cell/tianxuan/releases/download/desktop-v10.165.0/tianxuan-windows-amd64-installer.exe`
- **更新清单**: `https://github.com/wubitianxuan55-cell/tianxuan/releases/latest/download/latest.json`（HTTP 200，指向 v10.165.0）

## 本轮已交付能力（V10.160 → V10.165）

| 版本 | 提交 | 能力 |
|------|------|------|
| V10.160.0 | `be2b993` | OpenCode Zen 模型接入（opencode provider 三协议路由） |
| V10.161.0 | `fab8ad8` | 桌面端设置面板支持 OpenCode Zen |
| V10.162.0 | `4a9f9f7` | vision 识图技能（Zen 视觉模型外挂眼睛） |
| V10.163.0 | `a674aa8` | 修复更新安装时旧版本未关闭导致失败（NSIS 杀进程） |
| V10.164.0 | `6670e2e` | 截图→识图闭环（附件提示指向 vision 技能） |
| V10.165.0 | `d00d17e` | AI 自主截图工具 capture_screen（截图→识图全自动） |

完整视觉链路：用户说"截个屏看看" → AI 调 `capture_screen` 截主屏 →
`.tianxuan/attachments/screen-*.png` → 自动触发 vision 技能 →
OpenCode Zen 视觉模型（mimo-v2.5-free）返回描述 → AI 回答。

## 回滚方法

1. **回滚到发布版源码**：`git checkout desktop-v10.165.0`（或任意 `desktop-vX.Y.Z`）。
   所有 desktop tag 指向各自真实发布提交，已 force 修正并验证。
2. **回滚安装版**：安装版更新器只升不降，直接下载旧版本安装包覆盖安装
   （`releases/download/desktop-vX.Y.Z/...installer.exe`，V10.163.0 起安装器会自动
   关闭运行中的旧进程再覆盖）。
3. **架构调整期间**：每天至少一次 `git push origin release/v10.110.0`；
   每个里程碑打 `desktop-vX.Y.Z` tag 并验证远端指向
   （`git ls-remote --tags origin desktop-vX.Y.Z`）。

## 发布注意事项（踩过的坑）

- `gh release create` 默认以**默认分支 HEAD** 创建远端 tag——从 release 分支发布时
  必须传 `--target <commit>`（`publish-desktop.ps1` 已内置，勿删）。
- 发布脚本会 `robocopy .tianxuan/skills → tianxuan/internal/skill/bundled`（/E 不删），
  新技能/改技能必须同步到 bundled 再发布。
- NSIS `project.nsi` 有 UTF-8 BOM（makensis 按 ANSI 解析无 BOM 文件会报
  Bad text encoding）；nsExec 执行带重定向的命令必须 `cmd /c` 包裹。
- PowerShell 5.1 的 `Invoke-RestMethod` 会把无 charset 的 JSON 按 Latin-1 解码，
  vision.ps1 用 HttpWebRequest + 手动 UTF-8 解码。

## 环境与密钥

- OPENCODE_API_KEY / DEEPSEEK_API_KEY 在 `D:\AI\tianxuanX\.env`（不入库）
- 用户级配置 `%APPDATA%\tianxuan\config.toml`（zen provider 已预置）
- Codex 用户级技能 `C:\Users\Administrator\.codex\skills\vision\`（独立于 tianxuan）
