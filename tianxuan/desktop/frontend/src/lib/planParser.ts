/**
 * Hermes 计划内容解析器
 *
 * 从 Markdown 格式的 Hermes 计划文本中提取结构化步骤数据。
 * 格式模板见 tianxuan/internal/agent/hermes.go 的 "Step format" 节。
 *
 * 示例：
 *
 *   步骤 1：新增 planParser
 *   - **File(s)**：[NEW] lib/planParser.ts
 *   - **Change**：实现解析函数
 *   - **Depends on**：无
 *   - **Success**：npx vitest run 通过
 *   - **Risk recovery**：回退
 *
 * 解析失败时返回 null，由调用方降级为纯文本展示。
 */

export interface ParsedStep {
  number: number;
  title: string;
  files: string[];
  change: string;
  success: string;
  riskRecovery: string;
}

export interface ParsedPlan {
  steps: ParsedStep[];
  allFiles: string[];
}

// 步骤标题：步骤 N：标题 / 步骤 N: 标题
const STEP_HEADER_RE = /^步骤\s*(\d+)[：:]\s*(.+)$/gm;

// 文件名提取：去掉 [NEW] 前缀、反引号
function extractFiles(raw: string): string[] {
  // 匹配反引号包裹的路径或文件名
  const matches = raw.matchAll(/`([^`]+)`/g);
  const files: string[] = [];
  for (const m of matches) {
    const f = m[1].trim();
    if (f && !files.includes(f)) files.push(f);
  }
  return files;
}

/**
 * 解析 Hermes 计划 Markdown 文本。
 * 返回结构化数据，解析失败返回 null。
 */
export function parsePlan(plan: string): ParsedPlan | null {
  if (!plan || typeof plan !== "string") return null;

  const trimmed = plan.trim();
  const steps: ParsedStep[] = [];
  const allFilesSet = new Set<string>();

  // 使用 RegExp exec 遍历所有步骤标题
  let headerMatch: RegExpExecArray | null;
  const headers: { number: number; title: string; start: number; end: number }[] = [];

  // 第一遍：定位所有步骤标题及其范围
  STEP_HEADER_RE.lastIndex = 0;
  while ((headerMatch = STEP_HEADER_RE.exec(trimmed)) !== null) {
    headers.push({
      number: parseInt(headerMatch[1], 10),
      title: headerMatch[2].trim(),
      start: headerMatch.index,
      end: trimmed.length, // 临时，下一轮修正
    });
  }

  if (headers.length === 0) return null;

  // 修正每个步骤的结束位置为下一个步骤开始
  for (let i = 0; i < headers.length - 1; i++) {
    headers[i].end = headers[i + 1].start;
  }

  // 第二遍：解析每个步骤的字段
  for (const h of headers) {
    const block = trimmed.slice(h.start, h.end).trim();

    // 去掉步骤标题行本身
    const bodyLines: string[] = [];
    const lines = block.split("\n");
    for (const line of lines) {
      if (STEP_HEADER_RE.test(line.trim())) continue;
      bodyLines.push(line);
    }
    const body = bodyLines.join("\n").trim();

    // 提取各字段——从 body 中用 FIELD_RE 逐行匹配
    // 由于字段可能跨行（如 File(s) 有换行的子项），需用更灵活的方式
    let fileStr = "";
    let change = "";
    let success = "";
    let riskRecovery = "";

    // 按 - **Key**： 分割
    // 先按 bullet 行分割
    const fieldBlocks: { key: string; value: string }[] = [];
    const bulletLines = body.split("\n");
    let currentKey = "";
    let currentValue = "";

    for (const line of bulletLines) {
      const fm = line.match(/^- \*\*([^*]+)\*\*[：:]\s*(.*)$/);
      if (fm) {
        // 遇到新字段，保存上一个
        if (currentKey) {
          fieldBlocks.push({ key: currentKey, value: currentValue.trim() });
        }
        currentKey = fm[1].trim();
        currentValue = fm[2].trim();
      } else if (currentKey && line.trim()) {
        // 续行（如文件列表换行缩进）
        currentValue += " " + line.trim();
      }
    }
    if (currentKey) {
      fieldBlocks.push({ key: currentKey, value: currentValue.trim() });
    }

    for (const fb of fieldBlocks) {
      const key = fb.key.toLowerCase().replace(/[()]/g, "");
      switch (key) {
        case "files":
          fileStr = fb.value;
          break;
        case "change":
          change = fb.value;
          break;
        case "success":
          success = fb.value;
          break;
        case "risk recovery":
        case "riskrecovery":
          riskRecovery = fb.value;
          break;
      }
    }

    // 从 fileStr 提取文件名
    const stepFiles = extractFiles(fileStr);
    for (const f of stepFiles) allFilesSet.add(f);

    steps.push({
      number: h.number,
      title: h.title,
      files: stepFiles,
      change,
      success,
      riskRecovery,
    });
  }

  return {
    steps,
    allFiles: [...allFilesSet],
  };
}
