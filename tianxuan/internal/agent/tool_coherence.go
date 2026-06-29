package agent

import (
	"strings"

	"tianxuan/internal/provider"
)

// postBatchCoherenceCheck runs after a batch of tools executes. It detects
// patterns where a writer tool is doomed by an earlier failure in the same
// batch — for example, read_file("foo.go") failed but edit_file("foo.go")
// hasn't run yet. In those cases it replaces the doomed tool's result with
// a clear diagnostic, saving the model an extra roundtrip of "try → fail →
// understand why".
//
// 缓存安全: 仅修改本轮新产生的 tool_result 内容——这些消息尚未进入
// session.Messages 的历史前缀。下一次 API 调用时它们作为新消息追加，
// 不改变已有前缀。
func (a *AgentRunner) postBatchCoherenceCheck(calls []provider.ToolCall, results []string) {
	if len(calls) < 2 {
		return
	}

	// Phase 1: find files whose read/write failed in this batch.
	failedReads := map[string]string{}  // path → error
	failedWrites := map[string]string{} // path → error

	for i, c := range calls {
		path := extractFilePath(c.Name, c.Arguments)
		if path == "" {
			continue
		}
		if strings.HasPrefix(results[i], "error:") || strings.HasPrefix(results[i], "blocked:") ||
			strings.HasPrefix(results[i], "precheck blocked:") {
			if isReadTool(c.Name) {
				failedReads[path] = results[i]
			} else if isWriteTool(c.Name) {
				failedWrites[path] = results[i]
			}
		}
	}

	if len(failedReads) == 0 && len(failedWrites) == 0 {
		return
	}

	// Phase 2: for each subsequent tool in the batch that targets a failed file,
	// replace its result with a diagnostic that explains the dependency.
	for i, c := range calls {
		path := extractFilePath(c.Name, c.Arguments)
		if path == "" {
			continue
		}

		// A write after a failed read on the same file → guaranteed to fail.
		if isWriteTool(c.Name) {
			if errMsg, ok := failedReads[path]; ok {
				if results[i] == "" || !strings.HasPrefix(results[i], "error:") {
					results[i] = "blocked: cannot edit " + path +
						" — an earlier read of this file in the same batch failed: " +
						truncateStr(errMsg, 120) +
						". Re-read the file first with read_file, then retry the edit."
				}
			}
		}

		// A read after a failed write on the same file → read may get stale data.
		if isReadTool(c.Name) {
			if errMsg, ok := failedWrites[path]; ok {
				// Don't block the read, but append a warning.
				if !strings.Contains(results[i], "[coherence warning]") {
					results[i] = "[coherence warning: an earlier write to " + path +
						" failed (" + truncateStr(errMsg, 80) +
						") — this read may reflect pre-edit state]\n" + results[i]
				}
			}
		}
	}
}

// isReadTool reports whether a tool name is a read-only file inspection tool.
func isReadTool(name string) bool {
	switch name {
	case "read_file", "grep", "ls", "glob", "lsp_definition", "lsp_references", "lsp_hover":
		return true
	}
	return false
}

// isWriteTool reports whether a tool name is a file-modifying tool.
func isWriteTool(name string) bool {
	switch name {
	case "edit_file", "write_file", "multi_edit", "edit_lines", "delete_range", "delete_symbol":
		return true
	}
	return false
}

// extractFilePath is defined in agent.go.
