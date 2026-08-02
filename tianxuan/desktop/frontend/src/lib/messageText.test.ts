import { describe, expect, it } from "vitest";
import { displayUserText } from "./messageText";

describe("displayUserText", () => {
  it("keeps plain text unchanged", () => {
    expect(displayUserText("你好，帮我看看这段代码")).toBe("你好，帮我看看这段代码");
  });

  it("replaces attachment references with [image]", () => {
    expect(displayUserText("看图 @.tianxuan/attachments/a1b2c3 再回答")).toBe("看图 [image] 再回答");
  });

  it("replaces multiple attachment references", () => {
    expect(displayUserText("@.tianxuan/attachments/x @.tianxuan/attachments/y")).toBe("[image] [image]");
  });

  it("handles empty text", () => {
    expect(displayUserText("")).toBe("");
  });
});
