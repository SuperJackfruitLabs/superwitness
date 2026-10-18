import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render } from "@testing-library/svelte";
import { ApiError } from "../api";
import type { EvidenceRef, HistoryVerdict, Rubric } from "../types";

const getJSON = vi.fn();
const postJSON = vi.fn();
const real = vi.hoisted(() => ({ post: null as null | ((path: string, body?: unknown) => Promise<unknown>) }));
vi.mock("../api", async (orig) => {
  const mod = await orig<typeof import("../api")>();
  real.post = mod.postJSON;
  return { ...mod, getJSON: (...a: unknown[]) => getJSON(...a), postJSON: (...a: unknown[]) => postJSON(...a) };
});
import { session } from "../session.svelte";
import { evidenceSummary, MAX_COMMENT } from "../verdict/drawer";
import VerdictDrawer from "./VerdictDrawer.svelte";

const rub = (standard: string, scale: Rubric["recognised_scale"]): Rubric =>
  ({ id: standard, version: 1, standard, name: standard, scale: {}, recognised_scale: scale, created_by: "", created_at: "" }) as Rubric;
const rubrics = [
  rub("rubric:ok@1", { kind: "decision", options: ["pass", "fail"] }),
  rub("rubric:odd@1", null),
  rub("rubric:score@1", { kind: "score", min: 0, max: 1 }),
  rub("rubric:flat@1", { kind: "score", min: 1, max: 1 }),
  rub("rubric:tone@1", { kind: "label", labels: ["formal", "casual"] }),
  rub("rubric:summary@1", { kind: "text" }),
  rub("rubric:legacy@1", { kind: "score", min: 1, max: 5 }),
];
const doc = { run: { ref: "canary:run_01" }, attempts: [{ id: "attempt_a1" }] };

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const btn = (name: string) => [...document.querySelectorAll("button")].find((b) => b.textContent?.trim() === name) as HTMLButtonElement;
const save = () => btn("Save verdict") ?? btn("Saving…");
const selects = () => document.querySelectorAll<HTMLSelectElement>(".drawer select");
const setSelect = async (el: HTMLSelectElement, v: string) => { await fireEvent.change(el, { target: { value: v } }); await flush(); };
const click = async (el: Element) => { await fireEvent.click(el); await flush(); };
const text = () => document.querySelector(".drawer")!.textContent!;

type Props = { doc?: object; evidence?: EvidenceRef[]; revising?: HistoryVerdict; onclose?: () => void; onrecorded?: () => void };
async function open(props: Props = {}) {
  const v = render(VerdictDrawer, { doc, evidence: [], onclose: () => {}, onrecorded: () => {}, ...props });
  await flush();
  await flush();
  return v;
}
async function fillDecision() {
  await setSelect(selects()[1], "rubric:ok@1");
  await click(btn("pass"));
}

beforeEach(() => {
  getJSON.mockReset().mockResolvedValue({ rubrics });
  postJSON.mockReset().mockResolvedValue({});
  session.expired = false;
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  session.expired = false;
});

describe("VerdictDrawer", () => {
  it("is a labelled modal dialog titled Record verdict, Revise verdict when revising", async () => {
    await open();
    const d = document.querySelector(".drawer")!;
    expect(d.getAttribute("role")).toBe("dialog");
    expect(d.getAttribute("aria-modal")).toBe("true");
    expect(document.getElementById(d.getAttribute("aria-labelledby")!)?.textContent).toBe("Record verdict");
    expect(getJSON).toHaveBeenCalledWith("/v1/rubrics");
    cleanup();
    await open({ revising: { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: [] } as unknown as HistoryVerdict });
    expect(document.querySelector(".drawer h2")?.textContent).toBe("Revise verdict");
  });

  it("offers this run and each attempt as the subject", async () => {
    await open();
    expect([...selects()[0].options].map((o) => [o.value, o.textContent])).toEqual([["run|canary:run_01", "This run"], ["attempt|attempt_a1", "Attempt attempt_a1"]]);
    expect(selects()[1].options[0].textContent).toBe("Choose a rubric…");
  });

  it("lists an unsupported rubric disabled with a note", async () => {
    await open();
    const o = [...document.querySelectorAll("option")].find((x) => x.value === "rubric:odd@1")!;
    expect(o.disabled).toBe(true);
    expect(o.textContent).toBe("rubric:odd@1 (rubric:odd@1): scale not supported in the app");
  });

  it("says so when no rubric has a supported scale", async () => {
    getJSON.mockResolvedValue({ rubrics: [rub("rubric:odd@1", null)] });
    await open();
    expect(text()).toContain("No rubric has a scale the app supports. Add one with superwitness rubric-add.");
  });

  it("sends the exact body", async () => {
    await open({ evidence: ["aaaaaaaaaaaaaaaa"] });
    await fillDecision();
    await click(save());
    const [path, body] = postJSON.mock.calls[0];
    expect(path).toBe("/v1/verdicts");
    expect(body).toMatchObject({ subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, evidence_refs: ["aaaaaaaaaaaaaaaa"] });
    expect(body.idempotency_key).toMatch(/^[0-9a-f-]{36}$/);
    expect(body.supersedes).toBeUndefined();
    expect(body.comment).toBeUndefined();
  });

  it("sends an attempt subject and a comment", async () => {
    await open();
    await setSelect(selects()[0], "attempt|attempt_a1");
    await fillDecision();
    await fireEvent.input(document.querySelector(".drawer textarea")!, { target: { value: "Fine." } });
    await click(save());
    expect(postJSON.mock.calls[0][1]).toMatchObject({ subject_kind: "attempt", subject_ref: "attempt_a1", comment: "Fine." });
  });

  it("closes and tells the run view when the verdict is recorded", async () => {
    const onrecorded = vi.fn();
    await open({ onrecorded });
    await fillDecision();
    await click(save());
    expect(onrecorded).toHaveBeenCalledTimes(1);
  });

  it("a retry after a refusal reuses the same idempotency key", async () => {
    postJSON.mockRejectedValueOnce(new ApiError(503, "store_unavailable", "try again", true)).mockResolvedValue({});
    await open();
    await fillDecision();
    await click(save());
    expect(document.querySelector(".refusal")?.textContent).toBe("try again");
    expect(document.querySelector(".refusal")?.getAttribute("role")).toBe("alert");
    await click(save());
    expect(postJSON).toHaveBeenCalledTimes(2);
    expect(postJSON.mock.calls[1][1].idempotency_key).toBe(postJSON.mock.calls[0][1].idempotency_key);
  });

  it("a new drawer makes a new idempotency key", async () => {
    await open();
    await fillDecision();
    await click(save());
    cleanup();
    await open();
    await fillDecision();
    await click(save());
    expect(postJSON.mock.calls[1][1].idempotency_key).not.toBe(postJSON.mock.calls[0][1].idempotency_key);
  });

  it("a double click while saving sends one request, and says Saving…", async () => {
    let done!: () => void;
    postJSON.mockReturnValue(new Promise<void>((r) => (done = r)));
    await open();
    await fillDecision();
    const b = save();
    await click(b);
    await click(b);
    expect(postJSON).toHaveBeenCalledTimes(1);
    expect(b.disabled).toBe(true);
    expect(b.textContent?.trim()).toBe("Saving…");
    await act(async () => done());
  });

  it.each([
    ["unknown_rubric", "That rubric version no longer exists"],
    ["supersedes_not_found", "The verdict you're revising no longer exists"],
    ["supersedes_mismatch", "A revision must keep the same subject and rubric"],
    ["idempotency_conflict", "This form was already submitted with different values; reopen it"],
    ["already_superseded", "You've already revised this verdict"],
    ["self_judgement", "an agent may not judge a run it executed"],
  ])("shows the %s refusal in plain words", async (code, words) => {
    postJSON.mockRejectedValue(new ApiError(409, code, code === "self_judgement" ? words : "raw"));
    await open();
    await fillDecision();
    await click(save());
    expect(document.querySelector(".refusal")?.textContent).toBe(words);
    expect(save().disabled).toBe(false);
  });

  it("a 401 while saving ends the session (sign-in carries the path) and records nothing", async () => {
    postJSON.mockImplementation((p: string, b: unknown) => real.post!(p, b));
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { code: "unauthenticated", message: "sign in" } }), { status: 401 })));
    const onrecorded = vi.fn();
    const onclose = vi.fn();
    await open({ onrecorded, onclose });
    await fillDecision();
    await click(save());
    expect(session.expired).toBe(true);
    expect(onrecorded).not.toHaveBeenCalled();
    expect(onclose).not.toHaveBeenCalled();
    expect(postJSON).toHaveBeenCalledTimes(1);
  });

  it("a revision fixes subject and rubric, starts from the old value, and sends supersedes", async () => {
    const prev = { id: "vrd_1", subject_kind: "attempt", subject_ref: "attempt_a1", standard: "rubric:ok@1", value: { decision: "fail" }, comment: "old" } as unknown as HistoryVerdict;
    await open({ revising: prev });
    const sels = selects();
    expect(sels[0].disabled && sels[1].disabled).toBe(true);
    expect(sels[0].value).toBe("attempt|attempt_a1");
    expect(btn("fail").getAttribute("aria-pressed")).toBe("true");
    await click(save());
    expect(postJSON.mock.calls[0][1]).toMatchObject({ subject_kind: "attempt", subject_ref: "attempt_a1", supersedes: "vrd_1", value: { decision: "fail" }, comment: "old" });
  });

  it("keeps Save off until the score is moved, then sends it as {score}", async () => {
    await open();
    await setSelect(selects()[1], "rubric:score@1");
    expect(save().disabled).toBe(true);
    expect(document.querySelector("output")?.textContent).toBe("not set");
    const slider = document.querySelector('input[type="range"]') as HTMLInputElement;
    expect(slider.getAttribute("aria-valuetext")).toBe("not set");
    await fireEvent.input(slider, { target: { value: "0.75" } });
    await flush();
    expect(save().disabled).toBe(false);
    expect(document.querySelector("output")?.textContent).toBe("0.75");
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.75 });
  });

  it("a click on the untouched slider where it sits chooses that score", async () => {
    await open();
    await setSelect(selects()[1], "rubric:score@1");
    await click(document.querySelector('input[type="range"]')!);
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.5 });
  });

  it("rescales a legacy min..max score to 0..1 at four decimals", async () => {
    await open();
    await setSelect(selects()[1], "rubric:legacy@1");
    const slider = document.querySelector('input[type="range"]') as HTMLInputElement;
    expect([slider.min, slider.max, slider.step]).toEqual(["1", "5", "0.2"]);
    await fireEvent.input(slider, { target: { value: "2.4" } });
    await flush();
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.35 });
  });

  it("revising a score can be saved without moving it", async () => {
    const prev = { id: "vrd_2", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:score@1", value: { score: 0.3 }, comment: "", evidence_refs: [] } as unknown as HistoryVerdict;
    await open({ revising: prev });
    expect(save().disabled).toBe(false);
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.3 });
  });

  it("a label scale is a pick list and sends {label}", async () => {
    await open();
    await setSelect(selects()[1], "rubric:tone@1");
    const label = selects()[2];
    expect([...label.options].map((o) => o.textContent)).toEqual(["Choose…", "formal", "casual"]);
    expect(save().disabled).toBe(true);
    await setSelect(label, "casual");
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ label: "casual" });
  });

  it("a text scale is a text area and sends {text}, never blank", async () => {
    await open();
    await setSelect(selects()[1], "rubric:summary@1");
    const areas = document.querySelectorAll(".drawer textarea");
    expect(areas).toHaveLength(2);
    await fireEvent.input(areas[0], { target: { value: "   " } });
    await flush();
    expect(save().disabled).toBe(true);
    await fireEvent.input(areas[0], { target: { value: "Reads well." } });
    await flush();
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ text: "Reads well." });
  });

  it("lists a legacy score with an empty range as unsupported", async () => {
    await open();
    const o = [...document.querySelectorAll("option")].find((x) => x.value === "rubric:flat@1")!;
    expect(o.disabled).toBe(true);
    expect(o.textContent).toContain("scale not supported in the app");
  });

  it("counts cited transcript steps beside ticked spans", async () => {
    const step = { session_id: "acps_a1", seq_from: 3, seq_to: 4 };
    await open({ evidence: [step] });
    expect(text()).toContain("1 cited: 1 step from the transcript");
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

  it("says Evidence: none when nothing is cited", async () => {
    await open();
    expect(text()).toContain("Evidence: none; tick spans in the Trace tab or cite steps in the Transcript tab");
  });

  it("a revision keeps the earlier evidence, merged with spans ticked now, without repeats", async () => {
    const range = { session: "ses_01", from: 1, to: 3 };
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: ["aaaaaaaaaaaaaaaa", range] } as unknown as HistoryVerdict;
    await open({ revising: prev, evidence: ["bbbbbbbbbbbbbbbb", "aaaaaaaaaaaaaaaa"] });
    expect(text()).toContain("3 cited: 2 from the verdict being revised, 1 ticked in the Trace tab");
    await click(save());
    expect(postJSON.mock.calls[0][1].evidence_refs).toEqual(["aaaaaaaaaaaaaaaa", range, "bbbbbbbbbbbbbbbb"]);
  });

  it("a step the earlier verdict cites is not cited twice when the database returned its keys reordered", async () => {
    const stored = { seq_to: 4, seq_from: 3, session_id: "acps_a1" };
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: [stored] } as unknown as HistoryVerdict;
    await open({ revising: prev, evidence: [{ session_id: "acps_a1", seq_from: 3, seq_to: 4 }] });
    expect(text()).toContain("1 cited: 1 from the verdict being revised");
    expect(text()).not.toContain("from the transcript");
    await click(save());
    expect(postJSON.mock.calls[0][1].evidence_refs).toEqual([stored]);
  });

  it("a revision with no new ticks still cites the earlier evidence", async () => {
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: ["aaaaaaaaaaaaaaaa"] } as unknown as HistoryVerdict;
    await open({ revising: prev });
    expect(text()).toContain("1 cited: 1 from the verdict being revised");
    expect(text()).not.toContain("ticked in the Trace tab");
    await click(save());
    expect(postJSON.mock.calls[0][1].evidence_refs).toEqual(["aaaaaaaaaaaaaaaa"]);
  });

  it("refuses a revision whose merged evidence is over 100", async () => {
    const old = Array.from({ length: 60 }, (_, i) => i.toString(16).padStart(16, "a"));
    const now = Array.from({ length: 41 }, (_, i) => i.toString(16).padStart(16, "b"));
    const prev = { id: "vrd_1", subject_kind: "run", subject_ref: "canary:run_01", standard: "rubric:ok@1", value: { decision: "pass" }, comment: "", evidence_refs: old } as unknown as HistoryVerdict;
    await open({ revising: prev, evidence: now });
    expect(save().disabled).toBe(true);
    expect(text()).toContain("at most 100 spans and steps, the revised verdict's 60 included");
  });

  it("refuses a comment over the limit and shows a counter", async () => {
    await open();
    await fillDecision();
    expect(text()).toContain(`0 / ${MAX_COMMENT}`);
    await fireEvent.input(document.querySelector(".drawer textarea")!, { target: { value: "x".repeat(MAX_COMMENT + 1) } });
    await flush();
    expect(text()).toContain(`${MAX_COMMENT + 1} / ${MAX_COMMENT}`);
    expect(save().disabled).toBe(true);
  });

  it("counts characters, not UTF-16 units", async () => {
    await open();
    await fireEvent.input(document.querySelector(".drawer textarea")!, { target: { value: "😀😀" } });
    await flush();
    expect(text()).toContain(`2 / ${MAX_COMMENT}`);
  });

  it("refuses more than 100 evidence spans", async () => {
    await open({ evidence: Array.from({ length: 101 }, (_, i) => i.toString(16).padStart(16, "0")) });
    await fillDecision();
    expect(save().disabled).toBe(true);
    const alert = [...document.querySelectorAll('[role="alert"]')].find((a) => a.textContent?.includes("at most 100"))!;
    expect(alert.textContent).toBe("A verdict cites at most 100 spans and steps; untick spans in the Trace tab or uncite steps in the Transcript tab");
  });

  describe("focus and closing", () => {
    const key = (k: string, shiftKey = false) =>
      act(async () => void document.dispatchEvent(new KeyboardEvent("keydown", { key: k, shiftKey, bubbles: true, cancelable: true })));
    const drawer = () => document.querySelector(".drawer") as HTMLElement;
    const focusables = () => [...drawer().querySelectorAll<HTMLElement>("button, select, textarea, input, [href], [tabindex]")].filter((e) => !(e as HTMLButtonElement).disabled && e.tabIndex >= 0);

    it("moves focus into the drawer on open and back to the opener on close", async () => {
      const opener = document.createElement("button");
      document.body.append(opener);
      opener.focus();
      const v = await open();
      expect(drawer().contains(document.activeElement)).toBe(true);
      v.unmount();
      await flush();
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

    it("closes on Escape, the backdrop, Close and Cancel when idle", async () => {
      const onclose = vi.fn();
      await open({ onclose });
      await key("Escape");
      await click(document.querySelector(".backdrop")!);
      await click(document.querySelector('.drawer button[aria-label="Close"], .drawer .sw-drawer__x')!);
      await click(btn("Cancel"));
      expect(onclose).toHaveBeenCalledTimes(4);
    });

    it("does not close on Escape, the backdrop, Close or Cancel while a save is in flight", async () => {
      let done!: () => void;
      postJSON.mockReturnValue(new Promise<void>((r) => (done = r)));
      const onclose = vi.fn();
      await open({ onclose });
      await fillDecision();
      await click(save());
      await key("Escape");
      await click(document.querySelector(".backdrop")!);
      await click(document.querySelector(".drawer .sw-drawer__x")!);
      await click(btn("Cancel"));
      expect(onclose).not.toHaveBeenCalled();
      expect(btn("Cancel").disabled).toBe(true);
      expect(drawer().getAttribute("aria-busy")).toBe("true");
      await act(async () => done());
    });
  });
});
