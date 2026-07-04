// Package builtin provides Tianxuan's compile-time built-in tools. Each tool
// self-registers via init(); main blank-imports this package to wire them in.
package builtin

import "encoding/json"

// compactDesc maps tool names to single-line Chinese descriptions (~15-25 chars),
// used by CompactDescriptor to slash per-turn prompt tokens by ~75%.
var compactDesc = map[string]string{
	"read_file":      "读取文件(可选行范围/分页)",
	"edit_file":      "精确替换文件字符串(须全局唯一)",
	"write_file":     "写入/覆盖文件(自动建父目录)",
	"multi_edit":     "原子化批量编辑(单文件N步依次执行)",
	"edit_lines":     "按行号替换文件连续行(起止行号定位)",
	"delete_range":   "删除文件连续行(起止锚点定位)",
	"delete_symbol":  "删除Go符号(函数/类型/接口等,AST解析)",
	"glob":           "通配符匹配文件名(支持**递归)",
	"grep":           "正则搜索文件内容(返回path:行:文本,支持sort_by=relevance)",
	"ls":             "列目录条目(子目录带/)",
	"bash":           "执行shell命令(5分超时,output_format=json得结构化输出)",
	"bash_output":    "读取后台任务增量输出",
	"kill_shell":     "终止后台任务",
	"wait":           "阻塞等待后台任务结束",
	"web_fetch":      "抓取URL纯文本(去标签,SSRF安全,支持重试)",
	"web_search":     "搜索公开网页，返回结构化JSON(title/url/snippet/source)，支持引用追踪",
	"todo_write":     "更新任务清单(全量替换,最多一个进行中)",
	"complete_step":  "完成计划步骤(须可验证证据,禁止纯manual)",
	"notebook_edit":  "编辑Jupyter Notebook单元格(.ipynb)",
	"git_status":     "显示工作区状态(分支/暂存/未暂存/未跟踪/冲突)",
	"git_diff":       "显示行级别变更(--staged可选,path可限文件)",
	"git_commit":     "提交暂存变更(可stage_all/amend/自动生成消息)",
	"git_log":        "显示提交历史(支持count/path/author过滤)",
	"git_worktree":   "管理git工作树(添加/删除/列出)",
	"memory_search":  "搜索记忆(关键词+kind过滤,BM25排序)",
	"read_skill":     "读取指定技能(skill)的完整内容",
	"move_file":      "移动/重命名文件(自动建目录,工作区限制)",
	"code_index":     "轻量符号索引(outline/search,Go AST+多语言regex)",
}

// compactSchema maps tool names to stripped JSON Schema (properties without
// descriptions/constraints), used by CompactDescriptor.
var compactSchema = map[string]json.RawMessage{
	"read_file": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"},"line_numbers":{"type":"boolean"}},"required":["path"]}`),
	"edit_file": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}},"required":["path","old_string","new_string"]}`),
	"write_file": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
	"multi_edit": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["old_string","new_string"]}}},"required":["path","edits"]}`),
	"edit_lines": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer"},"end_line":{"type":"integer"},"new_content":{"type":"string"}},"required":["path","start_line","end_line","new_content"]}`),
	"delete_range": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"start_anchor":{"type":"string"},"end_anchor":{"type":"string"},"inclusive":{"type":"boolean"}},"required":["path","start_anchor","end_anchor"]}`),
	"delete_symbol": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"},"kind":{"type":"string"},"parent":{"type":"string"}},"required":["path","name"]}`),
	"move_file": json.RawMessage(
		`{"type":"object","properties":{"source_path":{"type":"string"},"destination_path":{"type":"string"}},"required":["source_path","destination_path"]}`),
	"glob": json.RawMessage(
		`{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"]}`),
	"grep": json.RawMessage(
		`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
	"ls": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"}}}`),
	"bash": json.RawMessage(
		`{"type":"object","properties":{"command":{"type":"string"},"run_in_background":{"type":"boolean"}},"required":["command"]}`),
	"bash_output": json.RawMessage(
		`{"type":"object","properties":{"job_id":{"type":"string"},"filter":{"type":"string"}},"required":["job_id"]}`),
	"kill_shell": json.RawMessage(
		`{"type":"object","properties":{"job_id":{"type":"string"}},"required":["job_id"]}`),
	"wait": json.RawMessage(
		`{"type":"object","properties":{"job_ids":{"type":"array","items":{"type":"string"}},"timeout_seconds":{"type":"integer"}}}`),
	"web_fetch": json.RawMessage(
		`{"type":"object","properties":{"url":{"type":"string"},"retries":{"type":"integer"}},"required":["url"]}`),
	"web_search": json.RawMessage(
		`{"type":"object","properties":{"query":{"type":"string"},"topK":{"type":"integer"}},"required":["query"]}`),
	"todo_write": json.RawMessage(
		`{"type":"object","properties":{"todos":{"type":"array","items":{"type":"object","properties":{"content":{"type":"string"},"status":{"type":"string"},"activeForm":{"type":"string"},"level":{"type":"integer"}},"required":["content","status"]}}},"required":["todos"]}`),
	"complete_step": json.RawMessage(
		`{"type":"object","properties":{"step":{"type":"string"},"step_index":{"type":"integer"},"result":{"type":"string"},"evidence":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string"},"summary":{"type":"string"},"command":{"type":"string"},"paths":{"type":"array","items":{"type":"string"}}},"required":["kind","summary"]}}},"required":["result","evidence"]}`),
	"notebook_edit": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string"},"cell_number":{"type":"integer"},"cell_id":{"type":"string"},"new_source":{"type":"string"},"cell_type":{"type":"string"},"edit_mode":{"type":"string"}},"required":["path"]}`),
	"git_status": json.RawMessage(
		`{"type":"object","properties":{},"required":[]}`),
	"git_diff": json.RawMessage(
		`{"type":"object","properties":{"staged":{"type":"boolean"},"path":{"type":"string"}}}`),
	"git_commit": json.RawMessage(
		`{"type":"object","properties":{"message":{"type":"string"},"stage_all":{"type":"boolean"},"amend":{"type":"boolean"}}}`),
	"git_log": json.RawMessage(
		`{"type":"object","properties":{"count":{"type":"integer"},"path":{"type":"string"},"author":{"type":"string"}}}`),
	"git_worktree": json.RawMessage(
		`{"type":"object","properties":{"action":{"type":"string"},"path":{"type":"string"},"branch":{"type":"string"},"base":{"type":"string"}},"required":["action"]}`),
	"memory_search": json.RawMessage(
		`{"type":"object","properties":{"query":{"type":"string"},"kind":{"type":"string"}},"required":["query"]}`),
	"read_skill": json.RawMessage(
		`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
	"code_index": json.RawMessage(
		`{"type":"object","properties":{"action":{"type":"string"},"path":{"type":"string"},"query":{"type":"string"},"kind":{"type":"string"},"limit":{"type":"integer"}},"required":["action"]}`),
}
