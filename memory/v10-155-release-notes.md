---
name: v10-155-release-notes
title: V10.155.0 版本发布
description: V10.155.0 版本发布 — Windows 安装包 + 自动更新链路打通（NSIS per-user 安装器、发布目标修复、minisign 密钥轮换）
---

## V10.155.0 版本发布

| 项 | 值 |
|------|------|
| 版本 | V10.155.0 |
| 日期 | 2026-08-05 |
| 范围 | desktop（updater / cmd/sign / internal/update / NSIS 模板）+ scripts/publish-desktop.ps1 + release-desktop.yml |
| 主题 | Windows 安装包 + 自动更新链路打通 |
| 产物 | `release/v10.155.0/tianxuan-windows-amd64-installer.exe` |
| 类型 | per-user NSIS 安装器（免管理员，完成页自动运行） |
| 大小 | 10,731,509 bytes (~10.2 MB) |
| SHA256 | `afb864475273053721d9bd48370f1f17b93d648ad6a244655081a76fccd55d19` |

### 主要变更

| 模块 | 变更 |
|------|------|
| 安装包 | `wails build -nsis` + 自定义 `project.nsi`：per-user 安装、完成页自动运行应用 |
| 发布目标 | updater / sign / workflow 的 `esengine/tianxuan` 与 Reasonix R2 残留 → `wubitianxuan55-cell/tianxuan`；R2 镜像改为 vars 配置 |
| 签名 | minisign 密钥轮换，新 key ID `154E38FBADA79807`（私钥加密存 `~/.tianxuan-release/`，不入 git） |
| 发布脚本 | `scripts/publish-desktop.ps1`：测试 → 构建 → 签名 → latest.json（含更新说明）→ GitHub Release |

### 验证

- desktop `go test ./...` 全绿 · `go vet` 干净
- `scripts/publish-desktop.ps1 -Version v10.155.0` 构建安装包 + minisign 签名 + manifest sha256 校验通过

### 下一步

- 用户运行一次 `gh auth login` 后，`scripts/publish-desktop.ps1 -Version vX.Y.Z` 即可推送更新
- 配置 R2 bucket（vars.R2_PUBLIC_BASE + secrets）后启用 CDN 镜像作为主端点
