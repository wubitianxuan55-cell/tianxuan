---
name: v10-145-release-notes
title: V10.145.0 发布记录
description: V10.145.0 发布记录 — 全面清理废弃残留（web 前端目录 + 根目录杂物）
---

## V10.145.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.145.0 |
| 发布日期 | 2026-08-01 |
| 代码变更 | 删除 tianxuan/web/（11 文件 4761 行）+ .gitignore +3 |
| 主题 | 全面清理废弃残留 |
| 构建产物 | `tianxuan-desktop.exe` |
| 文件大小 | 21,249,024 bytes (~20.3 MB) |
| SHA256 | `7aceb613cec2528285171c79d1ed976a7f230c8882cbfef92983da30df4b0385` |

### 本发布包含的提交

| 提交 | 主题 |
|------|------|
| (本轮) | chore(cleanup): 清理废弃 web 前端目录 + 根目录残留 + tools/ 加 gitignore |

### 清理内容

- 删除 `tianxuan/web/`（11 文件 4761 行）：V10.101 移动端移除后的废弃前端残留，
  零业务引用（serve 用 internal/serve/webui/）；含 bridge.ts Compact 键重复代码异味
- 删除根目录 `query`（360 卸载排查残留）与 `tianxuan-source.tar.gz`（43MB 备份）
- `tools/`（Go/Node 构建链）加入 .gitignore，防止误提交

### 验证

- `go build ./...` EXIT 0；desktop `go build ./...` EXIT 0
- `tsc --noEmit` 0 错误；vitest 61/61 全绿
