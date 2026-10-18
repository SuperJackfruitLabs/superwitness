// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api";
import type { HistoryVerdict, Rubric } from "../types";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("../api", async (orig) => ({
  ...(await orig<typeof import("../api")>()),
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));
import { evidenceSummary, MAX_COMMENT, VerdictDrawer } from "./Drawer";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const rub = (standard: string, scale: Rubric["recognised_scale"]): Rubric =>
  ({ id: standard, version: 1, standard, name: standard, scale: {}, recognised_scale: scale, created_by: "", created_at: "" }) as Rubric;
const rubrics = [
  rub("rubric:ok@1", { kind: "decision", options: ["pass", "fail"] }),
  rub("rubric:odd@1", null),
  rub("rubric:score@1", { kind: "score", min: 0, max: 1 }),
  rub("rubric:flat@1", { kind: "score", min: 1, max: 1 }),
];
const doc = { run: { ref: "canary:run_01" }, attempts: [{ id: "attempt_a1" }] };

let root: Root;
let host: HTMLElement;
const flush = () => act(async () => {});
const setSelect = (el: HTMLSelectElement, v: string) =>
  act(async () => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(el, v);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
const click = (el: Element) => act(async () => void el.dispatchEvent(new MouseEvent("click", { bubbles: true })));
const btn = (name: string) => [...host.querySelectorAll("button")].find((b) => b.textContent === name)!;
const save = () => btn("Save verdict") as HTMLButtonElement;

async function open(props: Partial<Parameters<typeof VerdictDrawer>[0]> = {}) {
  await act(async () => {
    root.render(<VerdictDrawer doc={doc} evidence={[]} onClose={() => {}} onRecorded={() => {}} {...props} />);
  });
  await flush();
}
async function fillDecision() {
  await setSelect(host.querySelectorAll("select")[1], "rubric:ok@1");
  await click(btn("pass"));
}

beforeEach(() => {
  getJSON.mockReset().mockResolvedValue({ rubrics });
  postJSON.mockReset().mockResolvedValue({});
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

describe("VerdictDrawer", () => {
  it("lists an unsupported rubric disabled with a note", async () => {
    await open();
    const o = [...host.querySelectorAll("option")].find((x) => x.value === "rubric:odd@1")!;
    expect(o.disabled).toBe(true);
    expect(o.textContent).toContain("scale not supported in the app");
  });

  it("sends the exact body", async () => {
    await open({ evidence: ["aaaaaaaaaaaaaaaa"] });
    await fillDecision();
    await click(save());
    const [path, body] = postJSON.mock.calls[0];
    expect(path).toBe("/v1/verdicts");
    expect(body).toMatchObject({
      subject_kind: "run",
      subject_ref: "canary:run_01",
      standard: "rubric:ok@1",
      value: { decision: "pass" },
      evidence_refs: ["aaaaaaaaaaaaaaaa"],
    });
    expect(body.idempotency_key).toMatch(/^[0-9a-f-]{36}$/);
    expect(body.supersedes).toBeUndefined();
  });

  it("a retry after a refusal reuses the same idempotency key", async () => {
    postJSON.mockRejectedValueOnce(new ApiError(503, "store_unavailable", "try again", true)).mockResolvedValue({});
    await open();
    await fillDecision();
    await click(save());
    expect(host.querySelector(".refusal")?.textContent).toBe("try again");
    await click(save());
    expect(postJSON).toHaveBeenCalledTimes(2);
    expect(postJSON.mock.calls[1][1].idempotency_key).toBe(postJSON.mock.calls[0][1].idempotency_key);
  });

  it("a double click while saving sends one request", async () => {
    let done!: () => void;
    postJSON.mockReturnValue(new Promise<void>((r) => (done = r)));
    await open();
    await fillDecision();
    const b = save();
    await click(b);
    await click(b);
    expect(postJSON).toHaveBeenCalledTimes(1);
    expect(b.disabled).toBe(true);
    await act(async () => done());
  });

  it("shows the already-revised refusal in plain words", async () => {
    postJSON.mockRejectedValue(new ApiError(409, "already_superseded", "raw"));
    await open();
    await fillDecision();
    await click(save());
    expect(host.querySelector(".refusal")?.textContent).toBe("You've already revised this verdict");
  });

  it("a revision fixes subject and rubric, starts from the old value, and sends supersedes", async () => {
    const prev = { id: "vrd_1", subject_kind: "attempt", subject_ref: "attempt_a1", standard: "rubric:ok@1", value: { decision: "fail" }, comment: "old" } as unknown as HistoryVerdict;
    await open({ revising: prev });
    const sels = host.querySelectorAll("select");
    expect(sels[0].disabled && sels[1].disabled).toBe(true);
    expect(btn("fail").getAttribute("aria-pressed")).toBe("true");
    await click(save());
    expect(postJSON.mock.calls[0][1]).toMatchObject({ subject_kind: "attempt", subject_ref: "attempt_a1", supersedes: "vrd_1", value: { decision: "fail" }, comment: "old" });
  });

  it("keeps Save off until the score is moved, then sends it as {score}", async () => {
    await open();
    await setSelect(host.querySelectorAll("select")[1], "rubric:score@1");
    expect(save().disabled).toBe(true);
    expect(host.querySelector("output")?.textContent).toBe("not set");
    const slider = host.querySelector('input[type="range"]') as HTMLInputElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(slider, "0.75");
      slider.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(save().disabled).toBe(false);
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.75 });
  });

  it("a click on the untouched slider where it sits chooses that score", async () => {
    await open();
    await setSelect(host.querySelectorAll("select")[1], "rubric:score@1");
    await click(host.querySelector('input[type="range"]')!);
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.5 });
  });

  it("revising a score can be saved without moving it", async () => {
    const prev = { id: "vrd_2", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:score@1", value: { score: 0.3 }, comment: "", evidence_refs: [] } as unknown as HistoryVerdict;
    await open({ revising: prev });
    expect(save().disabled).toBe(false);
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.3 });
  });

  it("lists a legacy score with an empty range as unsupported", async () => {
    await open();
    const o = [...host.querySelectorAll("option")].find((x) => x.value === "rubric:flat@1")!;
    expect(o.disabled).toBe(true);
    expect(o.textContent).toContain("scale not supported in the app");
  });

  it("counts cited transcript steps beside ticked spans", async () => {
    const step = { session_id: "acps_a1", seq_from: 3, seq_to: 4 };
    await open({ evidence: [step] });
    expect(host.textContent).toContain("1 cited: 1 step from the transcript");
    await fillDecision();
    await click(save());
    expect(postJSON.mock.calls[0][1].evidence_refs).toEqual([step]);
  });

  it("words the evidence line for every mix", () => {
    const step = { session_id: "acps_a1", seq_from: 3, seq_to: 4 };
    expect(evidenceSummary([], 0)).toBe("none; tick spans in the Trace tab or cite steps in the Transcript tab");
    expect(evidenceSummary(["aaaaaaaaaaaaaaaa"], 0)).toBe("1 span ticked in the Trace tab");
    expect(evidenceSummary(["aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"], 0)).toBe("2 spans ticked in the Trace tab");
    expect(evidenceSummary([step, step], 0)).toBe("2 cited: 2 steps from the transcript");
    expect(evidenceSummary(["aaaaaaaaaaaaaaaa", step], 0)).toBe("2 cited: 1 ticked in the Trace tab, 1 step from the transcript");
    expect(evidenceSummary(["aaaaaaaaaaaaaaaa", step], 1)).toBe("2 cited: 1 from the verdict being revised, 1 step from the transcript");
    expect(evidenceSummary(["aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"], 1)).toBe("2 cited: 1 from the verdict being revised, 1 ticked in the Trace tab");
    expect(evidenceSummary(["aaaaaaaaaaaaaaaa"], 1)).toBe("1 cited: 1 from the verdict being revised");
  });

  it("a revision keeps the earlier evidence, merged with spans ticked now, without repeats", async () => {
    const range = { session: "ses_01", from: 1, to: 3 };
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: ["aaaaaaaaaaaaaaaa", range] } as unknown as HistoryVerdict;
    await open({ revising: prev, evidence: ["bbbbbbbbbbbbbbbb", "aaaaaaaaaaaaaaaa"] });
    expect(host.textContent).toContain("3 cited: 2 from the verdict being revised, 1 ticked in the Trace tab");
    await click(save());
    expect(postJSON.mock.calls[0][1].evidence_refs).toEqual(["aaaaaaaaaaaaaaaa", range, "bbbbbbbbbbbbbbbb"]);
  });

  it("a step the earlier verdict cites is not cited twice when the database returned its keys reordered", async () => {
    const stored = { seq_to: 4, seq_from: 3, session_id: "acps_a1" };
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: [stored] } as unknown as HistoryVerdict;
    await open({ revising: prev, evidence: [{ session_id: "acps_a1", seq_from: 3, seq_to: 4 }] });
    expect(host.textContent).toContain("1 cited: 1 from the verdict being revised");
    expect(host.textContent).not.toContain("from the transcript");
    await click(save());
    expect(postJSON.mock.calls[0][1].evidence_refs).toEqual([stored]);
  });

  it("a revision with no new ticks still cites the earlier evidence", async () => {
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: ["aaaaaaaaaaaaaaaa"] } as unknown as HistoryVerdict;
    await open({ revising: prev });
    expect(host.textContent).toContain("1 cited: 1 from the verdict being revised");
    expect(host.textContent).not.toContain("ticked in the Trace tab");
    await click(save());
    expect(postJSON.mock.calls[0][1].evidence_refs).toEqual(["aaaaaaaaaaaaaaaa"]);
  });

  it("refuses a revision whose merged evidence is over 100", async () => {
    const old = Array.from({ length: 60 }, (_, i) => i.toString(16).padStart(16, "a"));
    const now = Array.from({ length: 41 }, (_, i) => i.toString(16).padStart(16, "b"));
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: old } as unknown as HistoryVerdict;
    await open({ revising: prev, evidence: now });
    expect(save().disabled).toBe(true);
    expect(host.textContent).toContain("at most 100 spans and steps, the revised verdict's 60 included");
  });

  it("refuses a comment over the limit and shows a counter", async () => {
    await open();
    await fillDecision();
    const ta = host.querySelector("textarea")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(ta, "x".repeat(MAX_COMMENT + 1));
      ta.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(host.textContent).toContain(`${MAX_COMMENT + 1} / ${MAX_COMMENT}`);
    expect(save().disabled).toBe(true);
  });

  it("refuses more than 100 evidence spans", async () => {
    await open({ evidence: Array.from({ length: 101 }, (_, i) => i.toString(16).padStart(16, "0")) });
    await fillDecision();
    expect(save().disabled).toBe(true);
    expect(host.textContent).toContain("at most 100 spans and steps");
  });

  describe("focus and closing", () => {
    const key = (k: string, shiftKey = false) =>
      act(async () => void document.dispatchEvent(new KeyboardEvent("keydown", { key: k, shiftKey, bubbles: true, cancelable: true })));
    const drawer = () => host.querySelector(".drawer") as HTMLElement;
    const focusables = () => [...drawer().querySelectorAll<HTMLElement>("button, select, textarea, input, [href], [tabindex]")].filter((e) => !(e as HTMLButtonElement).disabled && e.tabIndex >= 0);

    it("moves focus into the drawer on open and back to the opener on close", async () => {
      const opener = document.createElement("button");
      document.body.append(opener);
      opener.focus();
      await open();
      expect(drawer().contains(document.activeElement)).toBe(true);
      await act(async () => root.render(<div />));
      expect(document.activeElement).toBe(opener);
      opener.remove();
    });

    it("keeps Tab inside the drawer, wrapping at both ends", async () => {
      await open();
      const f = focusables();
      f[f.length - 1].focus();
      await key("Tab");
      expect(document.activeElement).toBe(f[0]);
      f[0].focus();
      await key("Tab", true);
      expect(document.activeElement).toBe(f[f.length - 1]);
    });

    it("pulls focus back in when it is outside the drawer", async () => {
      const outside = document.createElement("button");
      document.body.append(outside);
      await open();
      outside.focus();
      await key("Tab");
      expect(drawer().contains(document.activeElement)).toBe(true);
      outside.remove();
    });

    it("closes on Escape and on the backdrop when idle", async () => {
      const onClose = vi.fn();
      await open({ onClose });
      await key("Escape");
      await click(host.querySelector(".backdrop")!);
      expect(onClose).toHaveBeenCalledTimes(2);
    });

    it("does not close on Escape, the backdrop or Cancel while a save is in flight", async () => {
      let done!: () => void;
      postJSON.mockReturnValue(new Promise<void>((r) => (done = r)));
      const onClose = vi.fn();
      await open({ onClose });
      await fillDecision();
      await click(save());
      await key("Escape");
      await click(host.querySelector(".backdrop")!);
      await click(btn("Cancel"));
      expect(onClose).not.toHaveBeenCalled();
      await act(async () => done());
    });
  });
});
