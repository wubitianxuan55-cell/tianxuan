---
name: release
description: 一键发布 tianxuan 新版本 — 版本号同步、桌面端打包(内置排除清单)、CHANGELOG 模板、记忆更新。用户说"发布/打版/打包发布/出个新版本"时使用。
argument-hint: "[版本号]"
metadata:
  author: tianxuan
  version: "1.0.0"
runas: inline
allowed-tools: bash, read_file, write_file, edit_file, grep, ls, git_status, git_diff, git_commit, git_log
---

# 发布 (Release)

发布 tianxuan 新版本:版本号同步 → 全量验证 → 打包 → CHANGELOG → 记忆更新 → 提交。

## 何时使用

用户请求"发布 / 打版 / 打包发布 / 出个新版本"时激活。

## 0. 确定版本号

- 读 `memory/version-history.md` 表格第一行 = 当前版本。
- 新版本默认 = 当前版本次版本 + 1(`v10.152.0` → `v10.153.0`);用户显式给出则优先。
- 格式必须为 `v10.X.Y`,非法立即报错,不猜测。

## 1. 全量验证(带病不发布)

先跑,全部通过才继续:

- `go test ./...`
- `go vet ./...`

任一失败即停止,先修复再发布。

## 2. 版本号同步(4 处必须一致)

1. `release/CHANGELOG.md` 顶部插入新条目(模板见 §4)。
2. `tianxuan/CHANGELOG.md` 顶部插入代码变更条目(`## [10.X.Y] — YYYY-MM-DD`)。
3. 新建 `memory/v10-X-Y-release-notes.md`(模板见 §5)。
4. `memory/version-history.md` 表格第一行插入:`| V10.X.Y | YYYY-MM-DD | 一句话主题 |`。

## 3. 打包(scripts/package.ps1)

```powershell
powershell -ExecutionPolicy Bypass -File .tianxuan/skills/release/scripts/package.ps1 -Version v10.X.Y
```

脚本自动:

- 定位最新 `tianxuan-desktop.exe`;若其修改时间早于最近一次 git 提交,先重新构建(`build-desktop.bat`)。
- 复制到 `release/v10.X.Y/`,内置排除清单(不复制 .git/node_modules/build/.vite/dist/*.log)。
- 生成 `SHA256SUMS`(格式 `hash  tianxuan-desktop.exe`)。
- 输出 VERSION/EXE/BYTES/SIZE_MB/SHA256 摘要,供模板直接填写。

## 4. CHANGELOG 模板(release/CHANGELOG.md 顶部)

```markdown
## V10.X.Y (YYYY-MM-DD) — 一句话主题

### 变更
- ...

### 发布产物
- `release/v10.X.Y/tianxuan-desktop.exe` · N bytes (~X MB)
- SHA256: `...`
- 验证:`go test ./...` 全绿 · `go vet` 干净 · `wails build` EXIT 0

---
```

## 5. Release Notes 模板(memory/v10-X-Y-release-notes.md)

```markdown
---
name: v10-X-Y-release-notes
title: V10.X.Y 版本发布
description: V10.X.Y 版本发布 — 主题/范围/验证
---

## V10.X.Y 版本发布

| 项 | 值 |
|------|-----|
| 版本 | V10.X.Y |
| 日期 | YYYY-MM-DD |
| 范围 | ... |
| 主题 | ... |
| 产物 | `release/v10.X.Y/tianxuan-desktop.exe` |
| 大小 | N bytes (~X MB) |
| SHA256 | `...` |

### 主要变更

| 模块 | 变更 |
|------|------|
| ... | ... |

### 验证

- `go test ./...` 全绿;`go vet` 干净;`wails build` EXIT 0
- SHA256 已核对

### 下一步 5 件事

- ...
```

## 6. 提交

- `git add release/ memory/ tianxuan/`(若代码有变更)。
- 提交信息:`V10.X.Y 发布: 打包发布 + 文件记录 + 记忆更新`。
- 正文:发布范围、产物 SHA256、`release/v10.X.Y/ + SHA256SUMS + memory 基线/发布记录/版本历史`。
- 不推送、不打 tag,除非用户明确要求。

## 发布完成前逐项核对

- [ ] 4 处版本号一致(CHANGELOG ×2 / release-notes / version-history)
- [ ] `go test ./...` + `go vet ./...` 全绿
- [ ] SHA256SUMS 的 hash 与 exe 实际一致
- [ ] exe 修改时间 = 本次构建时间(不是旧产物)
- [ ] release-notes 的产物路径/大小/SHA256 与 `release/` 目录一致
