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
import { MAX_COMMENT, VerdictDrawer } from "./Drawer";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const rub = (standard: string, scale: Rubric["recognised_scale"]): Rubric =>
  ({ id: standard, version: 1, standard, name: standard, scale: {}, recognised_scale: scale, created_by: "", created_at: "" }) as Rubric;
const rubrics = [
  rub("rubric:ok@1", { kind: "decision", options: ["pass", "fail"] }),
  rub("rubric:odd@1", null),
  rub("rubric:score@1", { kind: "score", min: 0, max: 1 }),
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

  it("sends a score as {score}", async () => {
    await open();
    await setSelect(host.querySelectorAll("select")[1], "rubric:score@1");
    await click(save());
    expect(postJSON.mock.calls[0][1].value).toEqual({ score: 0.5 });
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
    expect(host.textContent).toContain("at most 100 spans");
  });
});
