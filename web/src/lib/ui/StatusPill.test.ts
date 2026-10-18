import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/svelte";
import StatusPill, { toneOf } from "./StatusPill.svelte";

afterEach(cleanup);

describe("StatusPill", () => {
  it.each([
    ["succeeded", "success"],
    ["completed", "success"],
    ["failed", "danger"],
    ["waiting", "warning"],
    ["running", "info"],
    ["queued", "neutral"],
    ["cancelled", "neutral"],
    ["unknown", "neutral"],
    ["something-else", "neutral"],
  ])("draws %s in the %s tone, with the word", (status, tone) => {
    expect(toneOf(status)).toBe(tone);
    const { container } = render(StatusPill, { status });
    const pill = container.querySelector(".sw-pill")!;
    expect(pill).toHaveAttribute("data-tone", tone);
    expect(pill.textContent?.trim()).toBe(status);
  });
  it("says unknown for an empty status, never a blank pill", () => {
    const { container } = render(StatusPill, { status: "" });
    expect(container.querySelector(".sw-pill")!.textContent?.trim()).toBe("unknown");
    expect(container.querySelector(".sw-pill")).toHaveAttribute("data-tone", "neutral");
  });
});
