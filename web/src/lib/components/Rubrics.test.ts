import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/svelte";
import { ApiError } from "../api";

const getJSON = vi.fn();
vi.mock("../api", async (orig) => ({ ...(await orig<typeof import("../api")>()), getJSON: (...a: unknown[]) => getJSON(...a) }));
import { describeScale } from "../rubrics";
import RubricPage from "./RubricPage.svelte";
import RubricsPage from "./RubricsPage.svelte";

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const release = { id: "release", version: 1, standard: "rubric:release@1", name: "Release note", scale: { kind: "decision", options: ["pass", "fail"] },
  recognised_scale: { kind: "decision", options: ["pass", "fail"] }, created_by: "prn_human01", created_at: "2026-10-04T09:00:00Z", body: "Does it say\n  what changed?" };
const custom = { ...release, id: "custom", standard: "rubric:custom@1", name: "Custom rubric", scale: { kind: "matrix" }, recognised_scale: null, created_at: "2026-10-13T09:00:00Z" };

beforeEach(() => { getJSON.mockReset(); });
afterEach(cleanup);

describe("describeScale", () => {
  it("says each scale", () => {
    expect(describeScale({ kind: "decision", options: ["pass", "fail"] })).toBe("A decision: pass, fail.");
    expect(describeScale({ kind: "label", labels: ["formal", "casual"] })).toBe("A label: formal, casual.");
    expect(describeScale({ kind: "score", min: 0, max: 1 })).toBe("A score from 0 to 1.");
    expect(describeScale({ kind: "score", min: 1, max: 5 })).toBe("A score from 1 to 5, recorded as 0 to 1.");
    expect(describeScale({ kind: "text" })).toBe("Free text.");
    expect(describeScale(null)).toBe("Not supported in the app.");
  });
});

describe("the Rubrics page", () => {
  it("lists each rubric: Name link, Rubric id@version, Created date, and marks an unsupported scale", async () => {
    getJSON.mockResolvedValue({ rubrics: [release, custom] });
    render(RubricsPage);
    await flush();
    expect(getJSON).toHaveBeenCalledWith("/v1/rubrics");
    expect(document.querySelector("h1")?.textContent).toBe("Rubrics");
    expect([...document.querySelectorAll("thead th")].map((t) => t.textContent)).toEqual(["Name", "Rubric", "Created"]);
    const rows = [...document.querySelectorAll("tbody tr")].map((r) => [...r.querySelectorAll("td")].map((d) => d.textContent));
    expect(rows).toEqual([["Release note", "release@1", "2026-10-04"], ["Custom rubric · scale not supported in the app", "custom@1", "2026-10-13"]]);
    expect(document.querySelector("tbody a")?.getAttribute("href")).toBe("/rubrics/release/1");
  });

  it("says when there are none", async () => {
    getJSON.mockResolvedValue({ rubrics: [] });
    render(RubricsPage);
    await flush();
    expect(document.body.textContent).toContain("No rubrics yet. Record one with superwitness rubric-add.");
    expect(document.querySelector("table")).toBeNull();
  });

  it("says Loading… while it reads", async () => {
    getJSON.mockImplementation(() => new Promise(() => {}));
    render(RubricsPage);
    await flush();
    expect(document.body.textContent).toContain("Loading…");
  });
});

describe("a rubric's page", () => {
  it("shows the rubric", async () => {
    getJSON.mockResolvedValue(release);
    render(RubricPage, { id: "release", version: 1 });
    await flush();
    expect(getJSON).toHaveBeenCalledWith("/v1/rubrics/release/1");
    const ref = document.querySelector(".sw-rubric__ref")!;
    expect(ref.textContent).toBe("Rubrics · rubric:release@1");
    expect(ref.querySelector("a")?.getAttribute("href")).toBe("/rubrics");
    expect(document.querySelector("h1")?.textContent).toBe("Release note");
    expect(document.body.textContent).toContain("Created 2026-10-04 by prn_human01");
    expect([...document.querySelectorAll("h2")].map((h) => h.textContent)).toEqual(["Scale", "Rubric"]);
    expect(document.body.textContent).toContain("A decision: pass, fail.");
    const pres = document.querySelectorAll("pre");
    expect(pres[0].textContent).toBe('{"kind":"decision","options":["pass","fail"]}');
    expect(pres[1].textContent).toBe("Does it say\n  what changed?");
  });

  it("says Rubric not found with a link to the rubrics", async () => {
    getJSON.mockImplementation(async () => { throw new ApiError(404, "rubric_not_found", "no rubric nope version 9"); });
    render(RubricPage, { id: "nope", version: 9 });
    await flush();
    expect(document.querySelector("h1")?.textContent).toBe("Rubric not found");
    expect([...document.querySelectorAll("a")].some((a) => a.getAttribute("href") === "/rubrics")).toBe(true);
  });
});
