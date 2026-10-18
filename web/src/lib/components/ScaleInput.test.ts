import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/svelte";
import type { Scale } from "../types";
import { initialInput } from "../verdict/scale";
import ScaleInput from "./ScaleInput.svelte";

const decision: Scale = { kind: "decision", options: ["pass", "fail"] };
const score: Scale = { kind: "score", min: 0, max: 1 };
const label: Scale = { kind: "label", labels: ["clear", "muddled"] };
const text: Scale = { kind: "text" };

afterEach(cleanup);

describe("ScaleInput", () => {
  it("decision: one toggle button per option in a Decision group", async () => {
    const onchange = vi.fn();
    render(ScaleInput, { scale: decision, input: { kind: "decision", choice: "fail" }, onchange });
    const group = document.querySelector("fieldset")!;
    expect(group.getAttribute("aria-label") ?? group.querySelector("legend")?.textContent).toBe("Decision");
    const bs = [...document.querySelectorAll("button")];
    expect(bs.map((b) => [b.textContent?.trim(), b.getAttribute("aria-pressed"), b.type])).toEqual([["pass", "false", "button"], ["fail", "true", "button"]]);
    await fireEvent.click(bs[0]);
    expect(onchange).toHaveBeenCalledWith({ kind: "decision", choice: "pass" });
  });

  it("score: a 0 to 1 slider in steps of 0.05 that says not set until touched", async () => {
    const onchange = vi.fn();
    render(ScaleInput, { scale: score, input: initialInput(score), onchange });
    const r = document.querySelector<HTMLInputElement>('input[type="range"]')!;
    expect([r.min, r.max, r.step, r.value]).toEqual(["0", "1", "0.05", "0.5"]);
    expect(r.getAttribute("aria-valuetext")).toBe("not set");
    expect(document.querySelector("output")?.textContent).toBe("not set");
    expect(document.querySelector("label")?.textContent).toContain("Score");
    await fireEvent.input(r, { target: { value: "0.25" } });
    expect(onchange).toHaveBeenLastCalledWith({ kind: "score", x: 0.25, touched: true });
  });

  it("score: touched shows the value with two decimals and no valuetext", () => {
    render(ScaleInput, { scale: score, input: { kind: "score", x: 0.6, touched: true }, onchange: () => {} });
    expect(document.querySelector("output")?.textContent).toBe("0.60");
    expect(document.querySelector('input[type="range"]')!.hasAttribute("aria-valuetext")).toBe(false);
  });

  it("label: a pick list labelled Label", async () => {
    const onchange = vi.fn();
    render(ScaleInput, { scale: label, input: initialInput(label), onchange });
    const s = document.querySelector("select")!;
    expect(s.closest("label")?.textContent).toContain("Label");
    expect([...s.options].map((o) => [o.value, o.textContent])).toEqual([["", "Choose…"], ["clear", "clear"], ["muddled", "muddled"]]);
    await fireEvent.change(s, { target: { value: "clear" } });
    expect(onchange).toHaveBeenLastCalledWith({ kind: "label", choice: "clear" });
    await fireEvent.change(s, { target: { value: "" } });
    expect(onchange).toHaveBeenLastCalledWith({ kind: "label", choice: null });
  });

  it("text: a text area labelled Verdict", async () => {
    const onchange = vi.fn();
    render(ScaleInput, { scale: text, input: initialInput(text), onchange });
    const t = document.querySelector("textarea")!;
    expect(t.closest("label")?.textContent).toContain("Verdict");
    await fireEvent.input(t, { target: { value: "ok" } });
    expect(onchange).toHaveBeenLastCalledWith({ kind: "text", text: "ok" });
  });
});
