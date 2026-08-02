import { describe, expect, it } from "vitest";
import {
  categorizeTool,
  filterCatalog,
  groupByCategory,
  highlightParts,
  sortCatalog,
  type CatalogTool,
} from "./toolCatalog";

function tool(name: string, description = "", readOnly = false): CatalogTool {
  return { name, description, fullDescription: description, readOnly };
}

describe("categorizeTool", () => {
  it("maps known tool names to their category", () => {
    expect(categorizeTool("read_file")).toBe("file");
    expect(categorizeTool("edit_lines")).toBe("file");
    expect(categorizeTool("bash")).toBe("command");
    expect(categorizeTool("bash_output")).toBe("command");
    expect(categorizeTool("git_status")).toBe("git");
    expect(categorizeTool("git_commit")).toBe("git");
    expect(categorizeTool("web_search")).toBe("network");
    expect(categorizeTool("todo_write")).toBe("plan");
    expect(categorizeTool("complete_step")).toBe("plan");
    expect(categorizeTool("ask")).toBe("plan");
    expect(categorizeTool("task")).toBe("subagent");
    expect(categorizeTool("explore")).toBe("subagent");
    expect(categorizeTool("run_skill")).toBe("skill");
    expect(categorizeTool("read_skill")).toBe("skill");
    expect(categorizeTool("remember")).toBe("memory");
    expect(categorizeTool("memory_search")).toBe("memory");
    expect(categorizeTool("code_index")).toBe("code");
    expect(categorizeTool("lsp_definition")).toBe("code");
    expect(categorizeTool("codegraph_search")).toBe("code");
  });

  it("falls back to other for unknown names", () => {
    expect(categorizeTool("some_future_tool")).toBe("other");
    expect(categorizeTool("")).toBe("other");
  });
});

describe("filterCatalog", () => {
  const tools = [
    tool("git_status", "显示工作区状态"),
    tool("bash", "执行 shell 命令"),
    tool("edit_file", "精确替换文件字符"),
  ];

  it("keeps everything when query is empty and onlyUsed is off", () => {
    expect(filterCatalog(tools, "", false, {})).toHaveLength(3);
  });

  it("matches by name (case-insensitive)", () => {
    expect(filterCatalog(tools, "GIT_STATUS", false, {}).map((t) => t.name)).toEqual(["git_status"]);
  });

  it("matches by description", () => {
    expect(filterCatalog(tools, "shell", false, {}).map((t) => t.name)).toEqual(["bash"]);
  });

  it("keeps only used tools when onlyUsed is on", () => {
    const counts = { bash: 2 };
    expect(filterCatalog(tools, "", true, counts).map((t) => t.name)).toEqual(["bash"]);
  });

  it("combines query and onlyUsed", () => {
    const counts = { git_status: 1, bash: 3 };
    expect(filterCatalog(tools, "bash", true, counts).map((t) => t.name)).toEqual(["bash"]);
    // "shell" only matches bash's description; bash is unused here -> filtered out.
    expect(filterCatalog(tools, "shell", true, { git_status: 1 })).toHaveLength(0);
  });
});

describe("sortCatalog", () => {
  it("puts used tools first and keeps relative order", () => {
    const tools = [
      tool("git_status"),
      tool("bash"),
      tool("edit_file"),
    ];
    const counts = { bash: 1, git_status: 3 };
    expect(sortCatalog(tools, counts).map((t) => t.name)).toEqual(["git_status", "bash", "edit_file"]);
  });
});

describe("groupByCategory", () => {
  it("groups in fixed category order and drops empty groups", () => {
    const tools = [
      tool("bash"),
      tool("read_file"),
      tool("web_search"),
      tool("mystery_tool"),
    ];
    const groups = groupByCategory(tools);
    expect(groups.map((g) => g.category)).toEqual(["file", "command", "network", "other"]);
    expect(groups[0].tools.map((t) => t.name)).toEqual(["read_file"]);
    expect(groups[3].tools.map((t) => t.name)).toEqual(["mystery_tool"]);
  });

  it("returns an empty list for no tools", () => {
    expect(groupByCategory([])).toEqual([]);
  });
});

describe("highlightParts", () => {
  it("splits text into hit and miss segments case-insensitively", () => {
    expect(highlightParts("Git Status", "git")).toEqual([
      { text: "Git", hit: true },
      { text: " Status", hit: false },
    ]);
  });

  it("handles repeated matches", () => {
    expect(highlightParts("bash → bash_output", "bash")).toEqual([
      { text: "bash", hit: true },
      { text: " → ", hit: false },
      { text: "bash", hit: true },
      { text: "_output", hit: false },
    ]);
  });

  it("returns the whole text unmarked when query is empty or unmatched", () => {
    expect(highlightParts("bash", "")).toEqual([{ text: "bash", hit: false }]);
    expect(highlightParts("bash", "zzz")).toEqual([{ text: "bash", hit: false }]);
  });
});
