// diffParser.ts — 解析内核返回的 unified diff（git diff -u 风格）为可渲染
// 的结构化行（Codex review pane 的 diff 视图基础）。纯函数，可测试。
export type DiffLineKind = "header" | "hunk" | "context" | "add" | "del";

export interface DiffLine {
  kind: DiffLineKind;
  text: string; // 去掉前缀（---/+++/@@/-/+/空格）后的原文
  oldLine?: number;
  newLine?: number;
}

const HUNK_RE = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

export function parseUnifiedDiff(diff: string): DiffLine[] {
  const raw = diff.trim();
  if (raw === "") return [];
  const out: DiffLine[] = [];
  let oldLine = 0;
  let newLine = 0;
  let inHunk = false;

  for (const line of raw.split("\n")) {
    const m = HUNK_RE.exec(line);
    if (m) {
      oldLine = Number(m[1]);
      newLine = Number(m[3]);
      inHunk = true;
      out.push({ kind: "hunk", text: `-${m[1]}${m[2] ? "," + m[2] : ""} +${m[3]}${m[4] ? "," + m[4] : ""}` });
      continue;
    }
    if (line.startsWith("--- ") || line.startsWith("+++ ")) {
      out.push({ kind: "header", text: line.slice(4) });
      continue;
    }
    if (!inHunk) {
      // hunk 之外的散行（防御）：按前缀分类但不追踪行号。
      if (line.startsWith("-") && !line.startsWith("---")) out.push({ kind: "del", text: line.slice(1) });
      else if (line.startsWith("+") && !line.startsWith("+++")) out.push({ kind: "add", text: line.slice(1) });
      else out.push({ kind: "header", text: line });
      continue;
    }
    if (line.startsWith("-")) {
      out.push({ kind: "del", text: line.slice(1), oldLine });
      oldLine++;
    } else if (line.startsWith("+")) {
      out.push({ kind: "add", text: line.slice(1), newLine });
      newLine++;
    } else {
      out.push({ kind: "context", text: line.slice(1), oldLine, newLine });
      oldLine++;
      newLine++;
    }
  }
  return out;
}
