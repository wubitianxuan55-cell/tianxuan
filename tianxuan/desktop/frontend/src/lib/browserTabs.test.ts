import { describe, expect, it } from "vitest";
import {
  addTab,
  createBrowserTab,
  hostInitial,
  removeTab,
  switchTab,
  tabTitle,
  updateTab,
  type BrowserTab,
} from "./browserTabs";
import { createBrowserHistory, pushHistory } from "./browserHistory";

describe("browserTabs", () => {
  it("creates a blank tab with an empty history stack", () => {
    const tab = createBrowserTab("t1");
    expect(tab.id).toBe("t1");
    expect(tab.history.entries).toEqual([]);
    expect(tab.history.index).toBe(-1);
    expect(tab.mode).toBe("page");
    expect(tab.input).toBe("");
    expect(tab.pendingHost).toBeNull();
  });

  it("adds a tab at the end and activates it", () => {
    const a = createBrowserTab("a");
    const { tabs, activeId } = addTab([a], "a");
    expect(tabs.length).toBe(2);
    expect(tabs[1].id).toBe(activeId);
    expect(activeId).not.toBe("a");
  });

  it("addTab keeps the original tabs untouched", () => {
    const a = createBrowserTab("a");
    const before = [a];
    addTab(before, "a");
    expect(before.length).toBe(1);
  });

  it("removes an inactive tab without changing the active one", () => {
    const a = createBrowserTab("a");
    const b = createBrowserTab("b");
    const c = createBrowserTab("c");
    const { tabs, activeId } = removeTab([a, b, c], "a", "b");
    expect(tabs.map((t) => t.id)).toEqual(["a", "c"]);
    expect(activeId).toBe("a");
  });

  it("removing the active tab activates its right neighbour, else the left one", () => {
    const a = createBrowserTab("a");
    const b = createBrowserTab("b");
    const c = createBrowserTab("c");
    const middle = removeTab([a, b, c], "b", "b");
    expect(middle.tabs.map((t) => t.id)).toEqual(["a", "c"]);
    expect(middle.activeId).toBe("c");

    const last = removeTab([a, b], "b", "b");
    expect(last.activeId).toBe("a");
  });

  it("removing the last tab empties the set", () => {
    const a = createBrowserTab("a");
    const { tabs, activeId } = removeTab([a], "a", "a");
    expect(tabs).toEqual([]);
    expect(activeId).toBe("");
  });

  it("ignores removal of an unknown id", () => {
    const a = createBrowserTab("a");
    const { tabs, activeId } = removeTab([a], "a", "nope");
    expect(tabs).toEqual([a]);
    expect(activeId).toBe("a");
  });

  it("switchTab activates a known id and rejects unknown ones", () => {
    const a = createBrowserTab("a");
    const b = createBrowserTab("b");
    expect(switchTab([a, b], "b")).toBe("b");
    expect(switchTab([a, b], "nope")).toBe("");
  });

  it("updateTab patches a tab immutably", () => {
    const a = createBrowserTab("a");
    const next = updateTab([a], "a", { input: "example.com" });
    expect(next[0].input).toBe("example.com");
    expect(a.input).toBe("");
    expect(next).not.toBe([a]);
  });

  it("tabTitle shows the current host, or empty for a blank tab", () => {
    const blank = createBrowserTab("blank");
    expect(tabTitle(blank)).toBe("");

    const loaded: BrowserTab = {
      ...createBrowserTab("loaded"),
      history: pushHistory(createBrowserHistory(), "https://docs.example.com/page"),
    };
    expect(tabTitle(loaded)).toBe("docs.example.com");
  });

  it("hostInitial returns the uppercased first letter of the hostname", () => {
    expect(hostInitial("https://example.com/path")).toBe("E");
    expect(hostInitial("http://localhost:3000")).toBe("L");
    expect(hostInitial("https://docs.example.com")).toBe("D");
  });

  it("hostInitial returns empty for blank or unparsable input", () => {
    expect(hostInitial("")).toBe("");
    expect(hostInitial("not a url")).toBe("");
  });
});
