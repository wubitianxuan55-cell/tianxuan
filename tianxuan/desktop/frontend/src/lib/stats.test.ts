import { describe, it, expect } from "vitest";
import {
  priceFor, calcCost, fmtTokens, fmtCost, fmtElapsed,
  hitRateColor, hitRate, aggSteps, colFromUsage,
  filterSteps, hitPct, withHitRate, mergeCols,
} from "./stats";
import type { StepRecord } from "./stats";

describe("priceFor", () => {
  it("returns flash prices for flash model", () => {
    const p = priceFor("deepseek-v4-flash");
    expect(p.cacheHit).toBe(0.0203);
    expect(p.input).toBe(1.015);
    expect(p.output).toBe(2.03);
  });

  it("returns pro prices for pro model", () => {
    const p = priceFor("deepseek-v4-pro");
    expect(p.cacheHit).toBe(0.0263);
    expect(p.input).toBe(3.154);
    expect(p.output).toBe(6.308);
  });

  it("falls back to default for unknown model", () => {
    const p = priceFor("gpt-4");
    expect(p.label).toBe("V4 Flash");
  });

  it("returns default for undefined label", () => {
    const p = priceFor(undefined);
    expect(p.label).toBe("V4 Flash");
  });
});

describe("calcCost", () => {
  it("calculates cost per million tokens", () => {
    expect(calcCost(1_000_000, 1.0)).toBeCloseTo(1.0, 4);
    expect(calcCost(500_000, 2.0)).toBeCloseTo(1.0, 4);
    expect(calcCost(0, 10)).toBe(0);
  });
});

describe("fmtTokens", () => {
  it("formats millions", () => {
    expect(fmtTokens(1_500_000)).toBe("1.5M");
  });
  it("formats thousands", () => {
    expect(fmtTokens(12_300)).toBe("12.3k");
    expect(fmtTokens(5_000)).toBe("5k");
  });
  it("formats small numbers as-is", () => {
    expect(fmtTokens(42)).toBe("42");
    expect(fmtTokens(0)).toBe("0");
  });
});

describe("fmtCost", () => {
  it("formats >= 0.01 with 2 decimals", () => {
    expect(fmtCost(0.123)).toBe("¥0.12");
    expect(fmtCost(1.5)).toBe("¥1.50");
  });
  it("formats < 0.01 with 4 decimals", () => {
    expect(fmtCost(0.001)).toBe("¥0.0010");
    expect(fmtCost(0.0001)).toBe("¥0.0001");
  });
  it("shows zero", () => {
    expect(fmtCost(0)).toBe("¥0");
  });
});

describe("fmtElapsed", () => {
  it("formats seconds", () => {
    expect(fmtElapsed(45_000)).toBe("45s");
  });
  it("formats minutes", () => {
    expect(fmtElapsed(125_000)).toBe("2m5s");
  });
});

describe("hitRateColor", () => {
  it("green >= 80", () => expect(hitRateColor(80)).toBe("text-ok"));
  it("yellow >= 50", () => expect(hitRateColor(60)).toBe("text-warning"));
  it("red < 50", () => expect(hitRateColor(30)).toBe("text-err"));
});

describe("hitRate", () => {
  it("clamps between 0 and 100", () => {
    expect(hitRate(50)).toBe(50);
    expect(hitRate(150)).toBe(100);
    expect(hitRate(-10)).toBe(0);
  });
});

describe("colFromUsage", () => {
  it("returns zeros for undefined", () => {
    const c = colFromUsage(undefined);
    expect(c.prompt).toBe(0);
    expect(c.cost).toBe(0);
  });

  it("computes cost from usage snapshot", () => {
    const c = colFromUsage({
      promptTokens: 1000, completionTokens: 500,
      cacheHitTokens: 800, cacheMissTokens: 200,
      totalTokens: 1500, sessionCacheHitTokens: 0, sessionCacheMissTokens: 0,
      costUsd: 0.005,
    });
    expect(c.prompt).toBe(1000);
    expect(c.completion).toBe(500);
    expect(c.cacheHit).toBe(800);
    expect(c.cacheMiss).toBe(200);
    expect(c.cost).toBe(0.005);
  });
});

describe("aggSteps", () => {
  const steps: StepRecord[] = [
    { step: 1, prompt: 100, completion: 50, cacheHit: 80, cacheMiss: 20, cost: 0.001, source: "main" },
    { step: 2, prompt: 200, completion: 100, cacheHit: 160, cacheMiss: 40, cost: 0.002, source: "subagent" },
  ];

  it("aggregates all steps", () => {
    const c = aggSteps(steps);
    expect(c.prompt).toBe(300);
    expect(c.completion).toBe(150);
    expect(c.cacheHit).toBe(240);
    expect(c.cacheMiss).toBe(60);
    expect(c.cost).toBeCloseTo(0.003, 5);
  });

  it("returns zeros for empty array", () => {
    const c = aggSteps([]);
    expect(c.prompt).toBe(0);
    expect(c.cost).toBe(0);
  });
});

describe("filterSteps", () => {
  const steps: StepRecord[] = [
    { step: 1, prompt: 100, completion: 50, cacheHit: 80, cacheMiss: 20, cost: 0.001, source: "main" },
    { step: 2, prompt: 200, completion: 100, cacheHit: 160, cacheMiss: 40, cost: 0.002, source: "subagent" },
    { step: 3, prompt: 150, completion: 75, cacheHit: 0, cacheMiss: 0 }, // no source → main
  ];

  it("returns all for 'all'", () => {
    expect(filterSteps(steps, "all").length).toBe(3);
  });

  it("filters main", () => {
    const r = filterSteps(steps, "main");
    expect(r.length).toBe(2);
    expect(r.every(s => !s.source || s.source !== "subagent")).toBe(true);
  });

  it("filters subagent", () => {
    const r = filterSteps(steps, "subagent");
    expect(r.length).toBe(1);
    expect(r[0].source).toBe("subagent");
  });
});

describe("hitPct", () => {
  it("computes percentage", () => {
    expect(hitPct(80, 20)).toBeCloseTo(80, 1);
    expect(hitPct(0, 0)).toBe(0);
    expect(hitPct(100, 0)).toBe(100);
  });
});

describe("withHitRate", () => {
  it("attaches hitPct to ColStats", () => {
    const c = { prompt: 100, completion: 50, cacheHit: 80, cacheMiss: 20, cost: 0.001 };
    const w = withHitRate(c);
    expect(w.hitPct).toBeCloseTo(80, 1);
  });
});

describe("mergeCols", () => {
  it("sums two ColStats", () => {
    const a = { prompt: 100, completion: 50, cacheHit: 80, cacheMiss: 20, cost: 0.001 };
    const b = { prompt: 200, completion: 100, cacheHit: 160, cacheMiss: 40, cost: 0.002 };
    const m = mergeCols(a, b);
    expect(m.prompt).toBe(300);
    expect(m.completion).toBe(150);
    expect(m.cacheHit).toBe(240);
    expect(m.cacheMiss).toBe(60);
    expect(m.cost).toBeCloseTo(0.003, 5);
  });
});
