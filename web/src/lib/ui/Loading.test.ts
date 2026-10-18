import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/svelte";
import Loading from "./Loading.svelte";

afterEach(cleanup);

describe("Loading", () => {
  it("says Loading… as a status", () => {
    render(Loading);
    expect(screen.getByRole("status").textContent).toContain("Loading…");
  });
  it("draws the app-level loading bare, with no placeholder bars", () => {
    const { container } = render(Loading, { bare: true });
    expect(screen.getByRole("status").textContent?.trim()).toBe("Loading…");
    expect(container.querySelector(".sw-loading__bars")).toBeNull();
  });
  it("keeps its placeholder bars hidden from screen readers", () => {
    const { container } = render(Loading);
    expect(container.querySelector(".sw-loading__bars")).toHaveAttribute("aria-hidden", "true");
  });
});
