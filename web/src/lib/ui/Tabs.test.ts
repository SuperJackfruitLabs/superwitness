import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { tick } from "svelte";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import Tabs, { tabEdges, tabScroll } from "./Tabs.svelte";

afterEach(cleanup);

const items = [
  { value: "trace", label: "Trace", href: "/r" },
  { value: "transcript", label: "Transcript", href: "/r?tab=transcript" },
  { value: "logs", label: "Logs", href: "/r?tab=logs" },
];

describe("Tabs", () => {
  it("draws each tab as a link with role tab and aria-selected", () => {
    render(Tabs, { label: "Run", items, value: "logs" });
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => [t.tagName, t.getAttribute("href"), t.getAttribute("aria-selected")])).toEqual([
      ["A", "/r", "false"],
      ["A", "/r?tab=transcript", "false"],
      ["A", "/r?tab=logs", "true"],
    ]);
    expect(screen.getByRole("tablist", { name: "Run" })).toBeTruthy();
  });
  it("Left, Right, Home and End move focus and activate the tab", async () => {
    const go = vi.fn();
    render(Tabs, { label: "Run", items, value: "trace", onactivate: go });
    const tabs = screen.getAllByRole("tab");
    await fireEvent.keyDown(tabs[0], { key: "ArrowRight" });
    expect(document.activeElement).toBe(tabs[1]);
    expect(go).toHaveBeenLastCalledWith("/r?tab=transcript");
    await fireEvent.keyDown(tabs[1], { key: "End" });
    expect(document.activeElement).toBe(tabs[2]);
    await fireEvent.keyDown(tabs[2], { key: "ArrowRight" });
    expect(document.activeElement).toBe(tabs[0]);
    await fireEvent.keyDown(tabs[0], { key: "ArrowLeft" });
    expect(document.activeElement).toBe(tabs[2]);
    await fireEvent.keyDown(tabs[2], { key: "Home" });
    expect(document.activeElement).toBe(tabs[0]);
    expect(go).toHaveBeenLastCalledWith("/r");
  });
});

// jsdom lays nothing out, so the strip is 358px showing 458px of tabs, each 76px wide, as on a phone.
describe("the tab strip on a narrow screen", () => {
  const six = ["trace", "transcript", "logs", "errors", "verdicts", "attempts"].map((v) => ({ value: v, label: v, href: `/r?tab=${v}` }));
  const keys = ["clientWidth", "scrollWidth", "offsetWidth", "offsetLeft"] as const;
  let saved: (PropertyDescriptor | undefined)[] = [];
  beforeEach(() => {
    saved = keys.map((k) => Object.getOwnPropertyDescriptor(HTMLElement.prototype, k));
    const strip = (e: HTMLElement) => e.getAttribute("role") === "tablist";
    Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return strip(this) ? 358 : 0; } });
    Object.defineProperty(HTMLElement.prototype, "scrollWidth", { configurable: true, get() { return strip(this) ? 458 : 0; } });
    Object.defineProperty(HTMLElement.prototype, "offsetWidth", { configurable: true, get() { return 76; } });
    Object.defineProperty(HTMLElement.prototype, "offsetLeft", { configurable: true, get() { return [...(this.parentElement?.children ?? [])].indexOf(this) * 76; } });
  });
  afterEach(() => {
    keys.forEach((k, i) => (saved[i] ? Object.defineProperty(HTMLElement.prototype, k, saved[i]!) : delete (HTMLElement.prototype as unknown as Record<string, unknown>)[k]));
  });
  const strip = () => screen.getByRole("tablist");

  it("scrolls the selected last tab into view and fades only the edge with tabs past it", async () => {
    render(Tabs, { label: "Run", items: six, value: "attempts" });
    await tick();
    expect(strip().scrollLeft).toBe(100);
    expect(strip().dataset.more).toBe("left");
  });

  it("leaves the first tab where it is and fades the right edge", async () => {
    render(Tabs, { label: "Run", items: six, value: "trace" });
    await tick();
    expect(strip().scrollLeft).toBe(0);
    expect(strip().dataset.more).toBe("right");
  });

  it("updates the fade as the strip scrolls", async () => {
    render(Tabs, { label: "Run", items: six, value: "trace" });
    await tick();
    strip().scrollLeft = 50;
    await fireEvent.scroll(strip());
    expect(strip().dataset.more).toBe("left right");
  });
});

describe("tab strip arithmetic", () => {
  it("fades the edges that have tabs past them", () => {
    expect(tabEdges(0, 358, 358)).toBeUndefined();
    expect(tabEdges(0, 358, 458)).toBe("right");
    expect(tabEdges(100, 358, 458)).toBe("left");
  });
  it("centres the selected tab when it can", () => {
    expect(tabScroll(152, 76, 358, 458)).toBe(11);
    expect(tabScroll(0, 76, 358, 458)).toBe(0);
    expect(tabScroll(380, 76, 358, 458)).toBe(100);
  });
});
