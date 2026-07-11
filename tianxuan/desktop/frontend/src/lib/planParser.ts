/**
 * Hermes 计划内容解析器
 *
 * 从 Markdown 格式的 Hermes 计划文本中提取结构化步骤数据。
 * 解析失败时返回 null，由调用方降级为纯文本展示。
 */

export interface ParsedStep {
  number: number;
  title: string;
  files: string[];
  change: string;
  dependsOn: string;
  success: string;
  riskRecovery: string;
}

export interface ParsedPlan {
  steps: ParsedStep[];
  allFiles: string[];
}

// 步骤标题正则 — 支持多种 LLM 输出变体：
//   步骤 1：标题          纯文本格式
//   ## 步骤 1：标题       Markdown h2
//   ### 步骤 1：标题      Markdown h3
//   **步骤 1**：标题     粗体包裹编号
const STEP_HEADER_RE = /^#{0,3}\s*步骤\s*(\d+)[：:—]\s*(.+)$/gm;
const LOOSE_STEP_RE = /^#{0,3}\s*(?:步骤|Step)\s*(\d+)\**\s*[：:—]\s*(.+)$/gm;

function extractFiles(raw: string): string[] {
  const matches = raw.matchAll(/`([^`]+)`/g);
  const files: string[] = [];
  for (const m of matches) { const f = m[1].trim(); if (f && !files.includes(f)) files.push(f); }
  return files;
}

export function parsePlan(plan: string): ParsedPlan | null {
  if (!plan || typeof plan !== "string") return null;

  let trimmed = plan.trim();
  if (trimmed.startsWith("<!--plan-->")) {
    trimmed = trimmed.slice("<!--plan-->".length).trim();
  }

  const steps: ParsedStep[] = [];
  const allFilesSet = new Set<string>();

  // Pass 1: find step headers (strict first, loose fallback)
  let headerMatch: RegExpExecArray | null;
  const headers: { number: number; title: string; start: number; end: number }[] = [];

  STEP_HEADER_RE.lastIndex = 0;
  while ((headerMatch = STEP_HEADER_RE.exec(trimmed)) !== null) {
    headers.push({ number: parseInt(headerMatch[1], 10), title: headerMatch[2].trim(), start: headerMatch.index, end: trimmed.length });
  }

  if (headers.length === 0 && /步骤\s*\d/.test(trimmed)) {
    LOOSE_STEP_RE.lastIndex = 0;
    while ((headerMatch = LOOSE_STEP_RE.exec(trimmed)) !== null) {
      headers.push({ number: parseInt(headerMatch[1], 10), title: headerMatch[2].trim(), start: headerMatch.index, end: trimmed.length });
    }
  }

  // Fallback 3: plain numbered list — "1. Title" or "1) Title" (with optional ##/### prefix)
  if (headers.length === 0) {
    const NUM_RE = /^#{0,3}\s*(\d+)[\.\)]\s+(.+)$/gm;
    let m: RegExpExecArray | null;
    while ((m = NUM_RE.exec(trimmed)) !== null) {
      headers.push({ number: parseInt(m[1], 10), title: m[2].trim(), start: m.index, end: trimmed.length });
    }
  }

  if (headers.length === 0) return null;

  for (let i = 0; i < headers.length - 1; i++) { headers[i].end = headers[i + 1].start; }

  // Pass 2: parse fields
  for (const h of headers) {
    const block = trimmed.slice(h.start, h.end).trim();
    const bodyLines: string[] = [];
    for (const line of block.split("\n")) {
      // Also skip lines that look like another step header (numbered list variant)
      const numMatch = line.trim().match(/^#{0,3}\s*(\d+)[\.\)]\s+/);
      if (STEP_HEADER_RE.test(line.trim()) || LOOSE_STEP_RE.test(line.trim()) || numMatch) continue;
      bodyLines.push(line);
    }
    const body = bodyLines.join("\n").trim();

    let fileStr = "", change = "", dependsOn = "", success = "", riskRecovery = "";
    const fieldBlocks: { key: string; value: string }[] = [];
    let currentKey = "", currentValue = "";

    for (const line of body.split("\n")) {
      // Match: - **Key**：value, **Key**: value, or **Key：** value (colon before/after bold closing)
      const fm = line.match(/(?:^-\s*)?\*\*([^*]+?)(?:\*\*\s*[：:—]|[：:—]\s*\*\*)\s*(.*)$/);
      if (fm) {
        if (currentKey) fieldBlocks.push({ key: currentKey, value: currentValue.trim() });
        currentKey = fm[1].trim();
        currentValue = fm[2].trim();
      } else if (currentKey && line.trim()) {
        currentValue += " " + line.trim();
      }
    }
    if (currentKey) fieldBlocks.push({ key: currentKey, value: currentValue.trim() });

    for (const fb of fieldBlocks) {
      const key = fb.key.toLowerCase().replace(/[()]/g, "");
      switch (key) {
        case "files": fileStr = fb.value; break;
        case "change": change = fb.value; break;
        case "depends on": case "dependson": dependsOn = fb.value; break;
        case "success": success = fb.value; break;
        case "risk recovery": case "riskrecovery": riskRecovery = fb.value; break;
      }
    }

    const stepFiles = extractFiles(fileStr);
    for (const f of stepFiles) allFilesSet.add(f);
    steps.push({ number: h.number, title: h.title, files: stepFiles, change, dependsOn, success, riskRecovery });
  }

  return { steps, allFiles: [...allFilesSet] };
}
