# V10.178.0 发布记录

日期：2026-08-13 · 类型：工程治理 + 代码质量梳理（无新功能）

## 变更摘要

### CI/CD 修复（此前从未生效）
- `.github` 从 `tianxuan/.github` 移到仓库根（GitHub 只识别根目录 `.github/`，
  此前的 workflow 全部未被执行过）
- `ci.yml` 触发分支 `main-v2`（不存在的 reasonix 时代分支）→ `main/master/release/v10.110.0`
- cacheguard 静态缓存守卫加入硬门禁（`go run ./cmd/cacheguard`）
- `go-version-file` 指向 `tianxuan/go.mod`，`working-directory: tianxuan`
- e2e-bot 清理 reasonix 遗留（cmd/reasonix→cmd/tianxuan、配置路径、变量名、go 版本）
- release / release-desktop workflow 修复嵌套后的脚本/模块路径

### 代码质量
- gofmt：124 个真实格式差异文件统一（import 排序 + struct 注释对齐）
- 死变量清理：`codegraph/projectmap.go` 的 `_ = mainGo`
- `hermes_prompt.go` → `solo_prompt.go`（内容本就是 SoloSystemPrompt）
- 4 处 `main-v2` 残留清零（.golangci.yml、pr-version-label.yml、2 个 ISSUE_TEMPLATE）
- `chat_tui.go` 首次拆分：7 个渲染函数移到 `render.go`（1907→1824 行）

### Bug 修复
- `learning.PruneOld`：`maxAgeDays` 原是死参数（注释说按 N 天过期但未实现），
  TDD 补全——LastSeen 早于 N 天删除、空 LastSeen 保守保留、小于等于 0 不过期
- `update`：Windows 更新回滚失败不再谎报"已恢复原二进制"，区分回滚成功/失败

### 文档与分支治理
- README/README.zh-CN/tianxuan README/V8.0-ARCHITECTURE 版本号 V10.147→V10.177
- AGENTS.md 版本记录补充 V10.175.1/V10.176.x/V10.177.0
- tianxuan.example.toml 11 处 reasonix→tianxuan
- default branch master→main，main 与 release/v10.110.0 同步，删除遗留 master
  （v10.87.0 有 tag 锚定）+ feat/release 历史分支（archive tag 保独有提交）

### 工作空间
- 清理约 1GB：release/v* exe 产物（294MB）+ 根 .codegraph（697MB）
- release/v*/ 加入 .gitignore，release 元数据（SHA256SUMS/latest.json/minisig）
  保留（git 跟踪）

## 验证
- cacheguard：no cache-safety issues
- go build / go vet / go test ./... 全绿
- 前端 vitest 187 例全绿（本轮未改前端）
- workflow YAML 结构解析正确

## 发布产物
- `release/v10.178.0/tianxuan-windows-amd64-installer.exe` · 10769941 bytes (~10.3 MB)
- SHA256: `4da0d5dee912838daa2ad5ce82fa31b2f5434ed617a414a4fa3f3be2fd91bca0`
- minisign 签名验证 OK · latest.json 匹配 · 远端 tag `desktop-v10.178.0`
- GitHub release: https://github.com/wubitianxuan55-cell/tianxuan/releases/tag/desktop-v10.178.0

## 相关提交
- 分支：release/v10.110.0（与 main 同步）
- 本轮 6 个提交：CI 修复 / e2e-bot / gofmt / 文档对齐 / 后端梳理 / 静默吞错
