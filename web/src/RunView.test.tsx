import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RunView } from "./RunPage";
import { emptyNote, isAuthStatus, list } from "./format";

const noop = () => {};

describe("RunView with nulls and unknowns", () => {
  const doc = {
    run: { ref: "superpipeline/b/r", card: { title: "unknown" }, stage: null, state: "unknown", agent: undefined, started_at: "unknown", ended_at: null },
    attempts: null,
    trace: { trace_ids: null, status: "unknown", sampled: "unknown" },
    errors: null,
    verdicts: [{ id: "v1", kind: "score", source: null, value: null, judge: null, judge_kind: null, standard: null, at: null }],
    cost: { status: "unknown", input_tokens: null, output_tokens: null, usd: null },
    log_count: "unknown",
    sources: { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "unavailable", errors: "unavailable", verdicts: "ok" },
  };

  it("does not throw on null arrays and shows nullable fields as a dash", () => {
    const html = renderToStaticMarkup(<RunView doc={doc} logs={null} logsError={null} onLoadLogs={noop} />);
    expect(html).toContain("stage <b>unknown</b>");
    expect(html).toContain("<td>unknown</td>");
  });

  it("does not claim 'none' when the errors source failed", () => {
    const html = renderToStaticMarkup(<RunView doc={doc} logs={null} logsError={null} onLoadLogs={noop} />);
    expect(html).toContain("unknown (source errors: unavailable)");
    expect(html).toContain("<h2>Errors</h2><p class=\"muted\">unknown (source errors: unavailable)</p>");
  });

  it("shows a logs-local error beside the document", () => {
    const html = renderToStaticMarkup(<RunView doc={doc} logs={null} logsError="HTTP 500" onLoadLogs={noop} />);
    expect(html).toContain("Could not load logs: HTTP 500");
    expect(html).toContain("<h2>Attempts</h2>");
  });
});

describe("RunView gate status", () => {
  const gate = (id: string, status: string) => ({ id, kind: "gate", source: "superpipeline", status, value: { decision: null },
    judge: "unknown", judge_kind: "unknown", standard: "stage:draft", at: "2026-10-04T10:45:00Z" });
  const doc = {
    run: { ref: "superpipeline/b/r", card: { title: "t" }, stage: "draft", state: "ended", agent: "a", started_at: "x", ended_at: "y" },
    attempts: [], trace: { trace_ids: [], status: "ok", sampled: "ok" }, errors: [],
    verdicts: [gate("gate_03", "pending"), gate("gate_04", "cancelled")],
    cost: { status: "unknown" }, log_count: "unknown",
    sources: { superpipeline: "ok", verdicts: "ok" },
  };
  it("renders a cancelled gate as cancelled, not pending", () => {
    const html = renderToStaticMarkup(<RunView doc={doc} logs={null} logsError={null} onLoadLogs={noop} />);
    expect(html).toContain("<th>status</th>");
    expect(html).toContain("<td>cancelled</td>");
    expect(html.match(/<td>pending<\/td>/g)?.length).toBe(1);
  });
});

describe("helpers", () => {
  it("list tolerates null", () => expect(list(null)).toEqual([]));
  it("emptyNote says none only when sources are ok", () => {
    expect(emptyNote({ errors: "ok" }, ["errors"])).toBe("none");
    expect(emptyNote({ errors: "timeout" }, ["errors"])).toBe("unknown (source errors: timeout)");
    expect(emptyNote({}, ["errors"])).toBe("unknown (source errors: unknown)");
  });
  it("isAuthStatus", () => {
    expect(isAuthStatus(401)).toBe(true);
    expect(isAuthStatus(403)).toBe(true);
    expect(isAuthStatus(500)).toBe(false);
  });
});
