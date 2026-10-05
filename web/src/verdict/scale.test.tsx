import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ApiError } from "../api";
import type { Scale } from "../types";
import { refusalText } from "./messages";
import { initialInput, stepOf, valueOf } from "./scale";
import { ScaleInput } from "./ScaleInput";

const decision: Scale = { kind: "decision", options: ["pass", "fail"] };
const score: Scale = { kind: "score", min: 0, max: 1 };
const legacy: Scale = { kind: "score", min: 1, max: 5 };
const label: Scale = { kind: "label", labels: ["clear", "muddled"] };
const text: Scale = { kind: "text" };
const noop = () => {};

describe("each scale kind", () => {
  it("decision: one button per option, sends {decision}", () => {
    const html = renderToStaticMarkup(<ScaleInput scale={decision} input={{ kind: "decision", choice: "fail" }} onChange={noop} />);
    expect(html).toContain('aria-pressed="false">pass</button>');
    expect(html).toContain('aria-pressed="true">fail</button>');
    expect(valueOf(decision, initialInput(decision))).toBeNull();
    expect(valueOf(decision, { kind: "decision", choice: "pass" })).toEqual({ decision: "pass" });
  });
  it("score: a 0 to 1 slider in steps of 0.05, sends {score}", () => {
    const html = renderToStaticMarkup(<ScaleInput scale={score} input={initialInput(score)} onChange={noop} />);
    expect(html).toContain('type="range" min="0" max="1" step="0.05"');
    expect(valueOf(score, { kind: "score", x: 0.75 })).toEqual({ score: 0.75 });
  });
  it("a legacy {min, max} score is rescaled to 0 to 1", () => {
    expect(stepOf(legacy)).toBe(0.2);
    expect(valueOf(legacy, { kind: "score", x: 4 })).toEqual({ score: 0.75 });
    expect(initialInput(legacy, { score: 0.25 })).toEqual({ kind: "score", x: 2 });
  });
  it("label: a pick list, sends {label}", () => {
    const html = renderToStaticMarkup(<ScaleInput scale={label} input={initialInput(label)} onChange={noop} />);
    expect(html).toContain('<option value="clear">clear</option>');
    expect(valueOf(label, { kind: "label", choice: "clear" })).toEqual({ label: "clear" });
  });
  it("text: a text area, sends {text}, refuses blank", () => {
    expect(renderToStaticMarkup(<ScaleInput scale={text} input={initialInput(text)} onChange={noop} />)).toContain("<textarea");
    expect(valueOf(text, { kind: "text", text: "  " })).toBeNull();
    expect(valueOf(text, { kind: "text", text: "Reads well." })).toEqual({ text: "Reads well." });
  });
  it("a revision starts from the earlier value", () => {
    expect(initialInput(decision, { decision: "pass" })).toEqual({ kind: "decision", choice: "pass" });
  });
});

describe("refusal messages", () => {
  it.each([
    ["unknown_rubric", "That rubric version no longer exists"],
    ["supersedes_not_found", "The verdict you're revising no longer exists"],
    ["supersedes_mismatch", "A revision must keep the same subject and rubric"],
    ["idempotency_conflict", "This form was already submitted with different values; reopen it"],
    ["already_superseded", "You've already revised this verdict"],
  ])("%s", (code, words) => {
    expect(refusalText(new ApiError(409, code, "server words"))).toBe(words);
  });
  it("shows any other code's message as is", () => {
    expect(refusalText(new ApiError(403, "self_judgement", "an agent may not judge a run it executed"))).toBe("an agent may not judge a run it executed");
  });
});
