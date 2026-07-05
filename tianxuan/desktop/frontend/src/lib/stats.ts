// stats.ts — pure computation helpers used by StatsPanel.
// Extracted for testability: no React, no DOM, no side effects.

import type { WireUsage } from "./types";

// ─── price table ───────────────────────────────────────────────

export const MODEL_PRICES: Record<string, { cacheHit: number; input: number; output: number; label: string }> = {
  "deepseek-v4-flash": { cacheHit: 0.0203, input: 1.015, output: 2.03, label: "V4 Flash" },
  "deepseek-v4-pro":   { cacheHit: 0.0263, input: 3.154, output: 6.308, label: "V4 Pro" },
};
const DEFAULT_PRICE = MODEL_PRICES["deepseek-v4-flash"];

export function priceFor(label?: string) {
  if (!label) return DEFAULT_PRICE;
  for (const [key, p] of Object.entries(MODEL_PRICES)) {
    if (label.includes(key)) return p;
  }
  return DEFAULT_PRICE;
}

export function calcCost(tokens: number, pricePerM: number): number {
  return (tokens / 1_000_000) * pricePerM;
}

// ─── formatting ─────────────────────────────────────────────────

export function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  return String(n);
}

export function fmtCost(v: number): string {
  if (v >= 0.01) return "¥" + v.toFixed(2);
  if (v > 0) return "¥" + v.toFixed(4);
  return "¥0";
}

export function fmtElapsed(ms: number): string {
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m${s % 60}s`;
}

// ─── hit rate ───────────────────────────────────────────────────

export function hitRateColor(rate: number): string {
  return rate >= 80 ? "text-ok" : rate >= 50 ? "text-warning" : "text-err";
}

export function hitRate(rate: number): number {
  return Math.min(100, Math.max(0, rate));
}

// ─── Step / Col aggregation ─────────────────────────────────────

export interface StepRecord {
  step: number;
  prompt: number;
  completion: number;
  cacheHit: number;
  cacheMiss: number;
  cost: number;
  source?: string;
}

export interface ColStats {
  prompt: number;
  completion: number;
  cacheHit: number;
  cacheMiss: number;
  cost: number;
}

/** Aggregate a list of steps into column stats (cost summed directly from StepRecord). */
export function aggSteps(steps: StepRecord[]): ColStats {
  let prompt = 0, completion = 0, cacheHit = 0, cacheMiss = 0, cost = 0;
  for (const s of steps) {
    prompt += s.prompt;
    completion += s.completion;
    cacheHit += s.cacheHit;
    cacheMiss += s.cacheMiss;
    cost += s.cost ?? 0;
  }
  return { prompt, completion, cacheHit, cacheMiss, cost };
}

/** Convert a WireUsage snapshot to column stats (cost from costUsd). */
export function colFromUsage(u: WireUsage | undefined): ColStats {
  if (!u) return { prompt: 0, completion: 0, cacheHit: 0, cacheMiss: 0, cost: 0 };
  return { prompt: u.promptTokens, completion: u.completionTokens, cacheHit: u.cacheHitTokens, cacheMiss: u.cacheMissTokens, cost: u.costUsd ?? 0 };
}

/** Filter steps by source tag. */
export function filterSteps(steps: StepRecord[], source: "main" | "subagent" | "all"): StepRecord[] {
  if (source === "all") return steps;
  const target = source === "subagent" ? "subagent" : "main";
  return steps.filter(s => s.source === target || (!s.source && source === "main"));
}

// ─── hit rate helpers for StatsPanel ────────────────────────────

export interface ColStatsWithRate extends ColStats {
  hitPct: number;
}

/** Compute hit rate from cacheHit/cacheMiss. */
export function hitPct(hit: number, miss: number): number {
  const total = hit + miss;
  return total > 0 ? (hit / total) * 100 : 0;
}

/** Attach hitPct to ColStats. */
export function withHitRate(c: ColStats): ColStatsWithRate {
  return { ...c, hitPct: hitPct(c.cacheHit, c.cacheMiss) };
}

/** Merge two ColStats into totals. */
export function mergeCols(a: ColStats, b: ColStats): ColStats {
  return {
    prompt: a.prompt + b.prompt,
    completion: a.completion + b.completion,
    cacheHit: a.cacheHit + b.cacheHit,
    cacheMiss: a.cacheMiss + b.cacheMiss,
    cost: a.cost + b.cost,
  };
}
