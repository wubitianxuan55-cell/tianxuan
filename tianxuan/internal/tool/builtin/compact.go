// Package builtin provides Tianxuan's compile-time built-in tools. Each tool
// self-registers via init(); main blank-imports this package to wire them in.
package builtin

import "encoding/json"

// compactDesc maps tool names to single-line Chinese descriptions (~15-25 chars),
// used by CompactDescriptor to slash per-turn prompt tokens by ~75%.
var compactDesc = map[string]string{
	"read_file":           "读取文件(可选行范围/分页)",
	"edit_file":           "精确替换字符串(replace_all可全部替换)",
	"apply_patch":         "补丁文本批量编辑(多文件,行级匹配)",
	"write_file":          "写入/覆盖文件(自动建父目录)",
	"multi_edit":          "原子化批量编辑(单文件N步依次执行)",
	"edit_lines":          "按行号替换连续行(起止行锚点校验,编辑后自动语法检查)",
	"delete_range":        "删除文件连续行(起止锚点定位)",
	"delete_symbol":       "删除Go符号(函数/类型/接口等,AST解析)",
	"glob":                "通配符匹配文件名(支持**递归)",
	"grep":                "正则搜索文件内容(path:行:文本,支持glob过滤/context_lines)",
	"ls":                  "列目录条目(子目录带/)",
	"bash":                "执行shell命令(宿主配置超时;timeout参数非法;长跑用run_in_background)",
	"bash_output":         "读取后台任务增量输出",
	"write_stdin":         "向交互式后台任务写入stdin输入",
	"kill_shell":          "终止后台任务",
	"wait":                "阻塞等待后台任务结束",
	"web_fetch":           "抓取URL纯文本(去标签,SSRF安全,支持重试)",
	"web_search":          "搜索公开网页，返回结构化JSON(title/url/snippet/source)，支持引用追踪",
	"todo_write":          "更新任务清单(全量替换,最多一个进行中,blocked=等外部依赖)",
	"complete_step":       "完成计划步骤(须可验证证据,禁止纯manual)",
	"notebook_edit":       "编辑Jupyter Notebook单元格(.ipynb)",
	"git_status":          "显示工作区状态(分支/暂存/未暂存/未跟踪/冲突)",
	"git_diff":            "显示行级别变更(--staged可选,path可限文件)",
	"git_commit":          "提交暂存变更(可stage_all/amend/自动生成消息)",
	"git_log":             "显示提交历史(支持count/path/author过滤)",
	"git_worktree":        "管理git工作树(添加/删除/列出)",
	"memory_search":       "搜索记忆(关键词+kind过滤,BM25排序)",
	"read_skill":          "读取指定技能(skill)的完整内容",
	"move_file":           "移动/重命名文件(自动建目录,工作区限制)",
	"code_index":          "轻量符号索引(outline/search,Go AST+多语言regex)",
	"search_large_output": "查询被卸载的大型工具输出(list/read/search)",
	"verify_gate":         "shell 验证门控(退出码裁决 pass/fail,确定性检查)",
}

// compactSchema maps tool names to stripped JSON Schema (properties without
// descriptions/constraints), used by CompactDescriptor.
var compactSchema = map[string]json.RawMessage{
	"read_file": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"offset":{"type":"integer","minimum":0,"description":"0-based line offset (default 0)"},"limit":{"type":"integer","minimum":1,"description":"Max lines to return (default 2000)"},"line_numbers":{"type":"boolean","description":"Prefix lines with 1-based numbers (default true)"}},"required":["path"]}`),
	"edit_file": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"old_string":{"type":"string","description":"Exact text to replace; must be unique unless replace_all"},"new_string":{"type":"string","description":"Replacement text (may be empty to delete)"},"replace_all":{"type":"boolean","description":"Replace every occurrence instead of requiring uniqueness"}},"required":["path","old_string","new_string"]}`),
	"apply_patch": json.RawMessage(
		`{"type":"object","properties":{"patch":{"type":"string","description":"Patch text (*** Begin Patch ... *** End Patch)"}},"required":["patch"]}`),
	"write_file": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"File path (parent dirs auto-created)"},"content":{"type":"string","description":"Full file content to write"}},"required":["path","content"]}`),
	"multi_edit": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"edits":{"type":"array","description":"Edits applied atomically in order","items":{"type":"object","properties":{"old_string":{"type":"string","description":"Exact text to replace"},"new_string":{"type":"string","description":"Replacement text"},"replace_all":{"type":"boolean","description":"Replace every occurrence"}},"required":["old_string","new_string"]}}},"required":["path","edits"]}`),
	"edit_lines": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"start_line":{"type":"integer","minimum":1,"description":"1-based start line (inclusive)"},"end_line":{"type":"integer","minimum":1,"description":"1-based end line (inclusive)"},"new_content":{"type":"string","description":"Replacement text for the line range"},"start_anchor":{"type":"string","description":"Expected exact content of start_line; mismatch rejects edit"},"end_anchor":{"type":"string","description":"Expected exact content of end_line; mismatch rejects edit"},"validate":{"type":"boolean","description":"Post-edit syntax check with rollback (default true)"}},"required":["path","start_line","end_line","new_content"]}`),
	"delete_range": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"start_anchor":{"type":"string","description":"Exact content of first line to delete"},"end_anchor":{"type":"string","description":"Exact content of last line to delete"},"inclusive":{"type":"boolean","description":"Whether end_anchor line is deleted too"}},"required":["path","start_anchor","end_anchor"]}`),
	"delete_symbol": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"name":{"type":"string","description":"Symbol name (function/type/interface)"},"kind":{"type":"string","description":"Optional symbol kind filter"},"parent":{"type":"string","description":"Optional enclosing symbol name"}},"required":["path","name"]}`),
	"move_file": json.RawMessage(
		`{"type":"object","properties":{"source_path":{"type":"string","description":"Current file path"},"destination_path":{"type":"string","description":"Target path (parent dirs auto-created)"}},"required":["source_path","destination_path"]}`),
	"glob": json.RawMessage(
		`{"type":"object","properties":{"pattern":{"type":"string","description":"Glob pattern (supports ** recursion)"}},"required":["pattern"]}`),
	"grep": json.RawMessage(
		`{"type":"object","properties":{"pattern":{"type":"string","description":"Regex to search for"},"path":{"type":"string","description":"Directory or file to search (default: workspace)"},"sort_by":{"type":"string","enum":["path","relevance"],"description":"Result ordering"}},"required":["pattern"]}`),
	"ls": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"Directory to list (default: cwd)"}}}`),
	"bash": json.RawMessage(
		`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"run_in_background":{"type":"boolean","description":"Run detached; returns job id; use for servers/watchers"},"output_format":{"type":"string","enum":["plain","json"],"description":"json returns {ok, exit_code, stdout, stderr}"}},"required":["command"]}`),
	"bash_output": json.RawMessage(
		`{"type":"object","properties":{"job_id":{"type":"string","description":"Background job id (e.g. bash-1)"},"filter":{"type":"string","description":"Regex; only matching lines returned"}},"required":["job_id"]}`),
	"kill_shell": json.RawMessage(
		`{"type":"object","properties":{"job_id":{"type":"string","description":"Background job id to terminate"}},"required":["job_id"]}`),
	"wait": json.RawMessage(
		`{"type":"object","properties":{"job_ids":{"type":"array","items":{"type":"string"},"description":"Job ids to wait for; omit waits for all"},"timeout_seconds":{"type":"integer","minimum":1,"description":"Max seconds to block before returning progress"}}}`),
	"web_fetch": json.RawMessage(
		`{"type":"object","properties":{"url":{"type":"string","description":"URL to fetch as plain text"},"retries":{"type":"integer","minimum":0,"description":"Retry count for transient failures (default 0)"}},"required":["url"]}`),
	"web_search": json.RawMessage(
		`{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"topK":{"type":"integer","minimum":1,"description":"Number of results (default 5)"}},"required":["query"]}`),
	"todo_write": json.RawMessage(
		`{"type":"object","properties":{"todos":{"type":"array","description":"Full todo list (replaces previous)","items":{"type":"object","properties":{"content":{"type":"string","description":"Task description"},"status":{"type":"string","enum":["pending","in_progress","completed","blocked"],"description":"Task state"},"activeForm":{"type":"string","description":"Active phrasing of an in_progress task"},"level":{"type":"integer","enum":[0,1],"description":"0=task, 1=subtask"}},"required":["content","status"]}}},"required":["todos"]}`),
	"complete_step": json.RawMessage(
		`{"type":"object","properties":{"step":{"type":"string","description":"Step name being completed"},"step_index":{"type":"integer","minimum":1,"description":"1-based step number in the plan"},"result":{"type":"string","description":"What was done"},"evidence":{"type":"array","description":"Verifiable evidence (manual rejected)","items":{"type":"object","properties":{"kind":{"type":"string","enum":["verification","diff","files","manual"],"description":"Evidence type"},"summary":{"type":"string","description":"What the evidence shows"},"command":{"type":"string","description":"Verification command run"},"paths":{"type":"array","items":{"type":"string"},"description":"Changed file paths"}},"required":["kind","summary"]}}},"required":["step","result","evidence"]}`),
	"notebook_edit": json.RawMessage(
		`{"type":"object","properties":{"path":{"type":"string","description":"Path to .ipynb file"},"cell_number":{"type":"integer","description":"0-based cell index"},"cell_id":{"type":"string","description":"Cell id (alternative to cell_number)"},"new_source":{"type":"string","description":"New cell source text"},"cell_type":{"type":"string","enum":["code","markdown"],"description":"Cell type to insert"},"edit_mode":{"type":"string","enum":["replace","insert","delete"],"description":"Edit operation"}},"required":["path"]}`),
	"git_status": json.RawMessage(
		`{"type":"object","properties":{},"required":[]}`),
	"git_diff": json.RawMessage(
		`{"type":"object","properties":{"staged":{"type":"boolean","description":"Show staged diff (default false)"},"path":{"type":"string","description":"Restrict diff to this path"}}}`),
	"git_commit": json.RawMessage(
		`{"type":"object","properties":{"message":{"type":"string","description":"Commit message"},"stage_all":{"type":"boolean","description":"Stage all changes first (default true)"},"amend":{"type":"boolean","description":"Amend the last commit"}}}`),
	"git_log": json.RawMessage(
		`{"type":"object","properties":{"count":{"type":"integer","description":"Number of commits to show (default 10)"},"path":{"type":"string","description":"Restrict to commits touching this path"},"author":{"type":"string","description":"Filter by author"}}}`),
	"git_worktree": json.RawMessage(
		`{"type":"object","properties":{"action":{"type":"string","enum":["add","remove","list"],"description":"Worktree operation"},"path":{"type":"string","description":"Worktree path"},"branch":{"type":"string","description":"Branch for the new worktree"},"base":{"type":"string","description":"Base ref for the new worktree"}},"required":["action"]}`),
	"memory_search": json.RawMessage(
		`{"type":"object","properties":{"query":{"type":"string","description":"Search keywords"},"kind":{"type":"string","enum":["semantic","episodic","procedural"],"description":"Memory kind filter"}},"required":["query"]}`),
	"read_skill": json.RawMessage(
		`{"type":"object","properties":{"name":{"type":"string","description":"Skill name to read in full"}},"required":["name"]}`),
	"code_index": json.RawMessage(
		`{"type":"object","properties":{"action":{"type":"string","enum":["outline","search"],"description":"outline lists symbols; search finds by query"},"path":{"type":"string","description":"File or directory to index"},"query":{"type":"string","description":"Symbol query for search action"},"kind":{"type":"string","description":"Symbol kind filter"},"limit":{"type":"integer","minimum":1,"description":"Max results"}},"required":["action"]}`),
	"search_large_output": json.RawMessage(
		`{"type":"object","properties":{"operation":{"type":"string","enum":["list","read","search"],"description":"list/read/search offloaded tool outputs"},"name":{"type":"string","description":"Offloaded output name"},"query":{"type":"string","description":"Search text for search operation"}},"required":["operation"]}`),
	"verify_gate": json.RawMessage(
		`{"type":"object","properties":{"checks":{"type":"array","description":"Verification commands; all must pass","items":{"type":"object","properties":{"name":{"type":"string","description":"Check name"},"command":{"type":"string","description":"Shell command to verify"},"timeout":{"type":"integer","minimum":1,"description":"Timeout seconds"}},"required":["name","command"]}}},"required":["checks"]}`),
}
