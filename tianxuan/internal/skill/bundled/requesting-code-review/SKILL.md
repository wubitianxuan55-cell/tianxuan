---
name: requesting-code-review
description: 完成任务、实现主要功能或合并前必须使用 — 派发代码审查子代理，按严重性分级处理反馈。蒸馏自 superpowers v5.1.0 requesting-code-review。
metadata:
  author: tianxuan (distilled from obra/superpowers v5.1.0)
  version: "1.0.0"
---

# 请求代码审查

## 何时审查（必须）

- 每个任务完成之后
- 主要功能实现完成
- 合并到主线之前

审查要早、要勤——问题越晚发现成本越高。

## 审查方式

1. **准备审查上下文**：变更摘要（改了什么）、需求/计划（应该做什么）、变更范围（git diff 或 base..HEAD）
2. **派发审查子代理**（review 工具）：
   - 输入只包含审查上下文，不携带本会话历史——让审查者聚焦工作产物，而不是你的思考过程
3. **按严重性处理反馈**：
   - 必须修复（Critical）→ 立即修复
   - 应该修复（Important）→ 继续工作前修复
   - 建议（Minor）→ 记录稍后处理
   - 反馈有误 → 基于技术理由反驳（拒绝谄媚，不表演性同意）

## 红线

- 合并前未审查，禁止合并
- 审查反馈不能静默忽略
- 修复后重新验证测试
