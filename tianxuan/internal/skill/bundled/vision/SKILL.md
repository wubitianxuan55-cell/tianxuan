---
name: vision
description: 让无视觉能力的模型（如 DeepSeek）也能看图：把用户提供的图片发给 OpenCode Zen 视觉模型（默认 mimo-v2.5-free），返回文字描述。用户发送图片/截图、收到 [image attachment] 附件引用（@.tianxuan/attachments/ 路径）、或询问"这张图里是什么/帮我看看这个截图/这个 UI 怎么样"时必须使用。
---

# Vision（识图技能）

主模型（如 DeepSeek）没有视觉输入能力，无法直接"看"图片。本技能把图片发给
OpenCode Zen 的视觉模型（`mimo-v2.5-free`，已验证支持 text/image 多模态），
由视觉模型返回文字描述，你再把这段描述当作"看到的画面"整合进回答。

## 何时使用

- 用户发送图片文件、截图路径、或粘贴的图片
- 用户问"这张图里是什么 / 帮我看看这个截图 / 这个 UI 怎么样 / 对比这两张图"
- 任何需要理解图像内容的场景

## 怎么用

脚本位于本 SKILL.md 同目录的 `scripts/vision.ps1`（即
`<本 SKILL.md 所在目录>\scripts\vision.ps1`；用户级路径通常是
`$HOME\.tianxuan\skills\vision\scripts\vision.ps1`）。用 bash 工具运行
（Windows 上 bash 工具就是 PowerShell）：

```powershell
powershell -ExecutionPolicy Bypass -File "<脚本绝对路径>" -Images "<图片路径1>","<图片路径2>" -Prompt "具体问题"
```

单张图可省略参数名（第一个位置参数就是图片路径）：

```powershell
powershell -ExecutionPolicy Bypass -File "<脚本绝对路径>" "C:\path\截图.png" -Prompt "这个界面是什么软件？指出所有按钮和报错信息"
```

把脚本 stdout 输出的文字描述当作"看到的画面"整合进你的回答；脚本出错时
非零退出并输出错误到 stderr（API 错误、图片不存在、缺 key 都会大声失败）。

## 配置

- 默认模型：`mimo-v2.5-free`（OpenCode Zen 免费线路，已验证可识图）
- API：`https://opencode.ai/zen/v1`（OpenAI 兼容格式）
- Key 来源（自动，按优先级）：环境变量 `VISION_API_KEY` → `OPENCODE_API_KEY`
  （tianxuan 主进程已从 `.env` 加载，脚本作为子进程直接继承）
- 可选参数：
  - `-Model gpt-5.4-nano`：付费模型，视觉理解更强（config.toml 已配）
  - `-MaxTokens 4096`：最大输出 token（默认 2048）
  - `-Api <URL>`：覆盖 API 端点（OpenAI 兼容）

## 安全规则（不可违反）

- 只把**用户明确给出的图片**发往视觉 API，绝不扫描、上传无关文件
- 脚本零依赖，只做"读图片 + 一个 HTTP POST"，不做任何其他操作
- 不打印、不写盘、不展示 API Key
- 图片超过 8MB 会提示但可继续（可能较慢）
