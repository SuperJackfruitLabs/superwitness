import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import Tabs from "./Tabs.svelte";

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
  it("scrolls the selected tab into view", () => {
    const spy = vi.fn();
    Element.prototype.scrollIntoView = spy;
    render(Tabs, { label: "Run", items, value: "logs" });
    expect(spy).toHaveBeenCalled();
    expect(spy.mock.contexts[0]).toBe(screen.getAllByRole("tab")[2]);
  });
});
