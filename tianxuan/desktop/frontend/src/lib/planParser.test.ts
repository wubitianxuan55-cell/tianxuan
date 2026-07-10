import { describe, it, expect } from "vitest";
import { parsePlan } from "./planParser";

const samplePlan = `
步骤 1：新增计划内容解析工具函数 \`parsePlan\`
- **File(s)**：[NEW] \`tianxuan/desktop/frontend/src/lib/planParser.ts\`
- **Change**：实现 \`parsePlan(plan: string)\` 函数，从 Markdown 计划文本中提取结构化数据
- **Depends on**：无
- **Success**：\`npx vitest run --reporter=verbose\` 中新增的 \`planParser.test.ts\` 全部通过
- **Risk recovery**：若正则不匹配真实 Hermes 输出格式，对照 \`hermes.go\` 中的步骤格式模板调整正则

步骤 2：重写 PlanCard 组件为结构化视图
- **File(s)**：\`tianxuan/desktop/frontend/src/components/PlanCard.tsx\`
- **Change**：替换现有 Markdown 文本块为结构化步骤卡片列表
- **Depends on**：步骤 1
- **Success**：\`npx tsc --noEmit\` 无新 TS 错误
- **Risk recovery**：若组件渲染异常，\`git checkout -- desktop/frontend/src/components/PlanCard.tsx\` 回退
`;

describe("parsePlan", () => {
  it("解析标准计划，返回正确结构", () => {
    const result = parsePlan(samplePlan);
    expect(result).not.toBeNull();
    expect(result!.steps.length).toBe(2);
    expect(result!.allFiles.length).toBeGreaterThan(0);
  });

  it("提取步骤 1 的信息", () => {
    const result = parsePlan(samplePlan)!;
    const step = result.steps[0];
    expect(step.number).toBe(1);
    expect(step.title).toContain("新增计划内容解析工具函数");
    expect(step.files).toContain("tianxuan/desktop/frontend/src/lib/planParser.ts");
    expect(step.change).toContain("parsePlan");
    expect(step.dependsOn).toBe("无");
    expect(step.success).toContain("npx vitest run");
    expect(step.riskRecovery).toContain("hermes.go");
  });

  it("提取步骤 2 的信息", () => {
    const result = parsePlan(samplePlan)!;
    const step = result.steps[1];
    expect(step.number).toBe(2);
    expect(step.title).toContain("重写 PlanCard");
    expect(step.files).toContain("tianxuan/desktop/frontend/src/components/PlanCard.tsx");
    expect(step.dependsOn).toBe("步骤 1");
    expect(step.success).toContain("npx tsc --noEmit");
    expect(step.riskRecovery).toContain("git checkout");
  });

  it("提取全局文件列表，无重复", () => {
    const result = parsePlan(samplePlan)!;
    expect(result.allFiles).toContain("tianxuan/desktop/frontend/src/lib/planParser.ts");
    expect(result.allFiles).toContain("tianxuan/desktop/frontend/src/components/PlanCard.tsx");
    // 去重
    const unique = new Set(result.allFiles);
    expect(unique.size).toBe(result.allFiles.length);
  });

  it("空字符串返回 null", () => {
    expect(parsePlan("")).toBeNull();
  });

  it("不含步骤标题的文本返回 null", () => {
    expect(parsePlan("这是一段普通文本\n没有步骤标题")).toBeNull();
  });

  it("含 [NEW] 标记的文件正确提取", () => {
    const result = parsePlan(samplePlan)!;
    const step = result.steps[0];
    expect(step.files[0]).toBe("tianxuan/desktop/frontend/src/lib/planParser.ts");
    expect(step.files[0]).not.toContain("[NEW]");
  });

  it("处理步骤间存在空行/分隔线", () => {
    const text = `
步骤 1：第一个步骤
- **File(s)**：\`file1.ts\`
- **Change**：修改 A
- **Depends on**：无
- **Success**：测试通过
- **Risk recovery**：回退

---

步骤 2：第二个步骤
- **File(s)**：\`file2.ts\`
- **Change**：修改 B
- **Depends on**：步骤 1
- **Success**：构建通过
- **Risk recovery**：回退
`;
    const result = parsePlan(text)!;
    expect(result.steps.length).toBe(2);
    expect(result.steps[0].number).toBe(1);
    expect(result.steps[1].number).toBe(2);
  });

  it("字段顺序不一致时仍能正确解析", () => {
    const text = `
步骤 1：乱序字段
- **Risk recovery**：回退
- **Success**：测试通过
- **Change**：修改内容
- **File(s)**：\`/path/to/file.ts\`
- **Depends on**：无
`;
    const result = parsePlan(text)!;
    const step = result.steps[0];
    expect(step.change).toBe("修改内容");
    expect(step.success).toBe("测试通过");
    expect(step.riskRecovery).toBe("回退");
    expect(step.files).toContain("/path/to/file.ts");
  });

  it("使用英文冒号 : 也能正确解析", () => {
    const text = `
步骤 1: 英文冒号标题
- **File(s)**: \`main.go\`
- **Change**: 修改逻辑
- **Depends on**: 无
- **Success**: go test 通过
- **Risk recovery**: reset
`;
    const result = parsePlan(text)!;
    expect(result.steps.length).toBe(1);
    expect(result.steps[0].title).toBe("英文冒号标题");
    expect(result.steps[0].change).toBe("修改逻辑");
    expect(result.steps[0].success).toBe("go test 通过");
  });

  it("使用 em dash — 作为步骤标题分隔符也能正确解析", () => {
    const text = `
步骤 1—使用 em dash 的标题
- **File(s)**：\`main.go\`
- **Change**：修改内容
- **Depends on**：无
- **Success**：go test 通过
- **Risk recovery**：reset
`;
    const result = parsePlan(text)!;
    expect(result.steps.length).toBe(1);
    expect(result.steps[0].title).toBe("使用 em dash 的标题");
  });

  it("字段使用 — (em dash) 作为分隔符也能正确解析", () => {
    const text = `
步骤 1：em dash 字段分隔符
- **File(s)** — \`main.go\`
- **Change** — 修改核心逻辑
- **Depends on** — 无
- **Success** — \`go test -run TestX\`
- **Risk recovery** — \`git checkout -- main.go\`
`;
    const result = parsePlan(text)!;
    expect(result.steps.length).toBe(1);
    expect(result.steps[0].change).toBe("修改核心逻辑");
    expect(result.steps[0].success).toBe("`go test -run TestX`");
    expect(result.steps[0].riskRecovery).toBe("`git checkout -- main.go`");
    expect(result.steps[0].files).toContain("main.go");
  });
});
