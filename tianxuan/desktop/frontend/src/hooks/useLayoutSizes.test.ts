import { describe, expect, it } from "vitest";
import {
  BROWSER_PANEL_DEFAULT_WIDTH,
  BROWSER_PANEL_MAX_WIDTH,
  BROWSER_PANEL_MIN_WIDTH,
  clampBrowserPanelWidth,
} from "./useLayoutSizes";

describe("clampBrowserPanelWidth", () => {
  it("keeps a default width unchanged on a normal window", () => {
    expect(clampBrowserPanelWidth(BROWSER_PANEL_DEFAULT_WIDTH, 264, 1440)).toBe(560);
  });

  it("never goes below the minimum width", () => {
    expect(clampBrowserPanelWidth(100, 264, 1440)).toBe(BROWSER_PANEL_MIN_WIDTH);
  });

  it("allows widening up to the absolute max on a large window", () => {
    // 1920 viewport: 62% ratio = 1190, chat guard = 1456 → absolute max 1080.
    expect(clampBrowserPanelWidth(1200, 264, 1920)).toBe(BROWSER_PANEL_MAX_WIDTH);
  });

  it("respects the viewport ratio cap on medium windows", () => {
    // 1440 viewport: 62% = 892 → 1000 clamps to 892.
    expect(clampBrowserPanelWidth(1000, 264, 1440)).toBe(892);
  });

  it("keeps the chat pane readable (min 200px) on narrow windows", () => {
    // 900 viewport with 264 sidebar: chat guard = 900-264-200 = 436,
    // ratio guard = 558 → 436 wins.
    expect(clampBrowserPanelWidth(800, 264, 900)).toBe(436);
  });

  it("falls back to the minimum when the window is too small for a wider pane", () => {
    expect(clampBrowserPanelWidth(600, 264, 700)).toBe(BROWSER_PANEL_MIN_WIDTH);
  });

  it("rounds fractional widths", () => {
    expect(clampBrowserPanelWidth(560.6, 264, 1440)).toBe(561);
  });
});
