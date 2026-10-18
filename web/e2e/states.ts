import type { Page } from "@playwright/test";

// The states the sweep visits, from the superwitness inventory (every route, and every state it
// names that the stub hub, the fake sources and a scripted /v1 answer through Playwright's `route`
// can reach). Not here: the Go server's own sign-in pages (outside the app) and states that need a
// second source the fake mode does not have beyond what a scripted answer gives.

export type PageType = "edge" | "shell" | "runs" | "run" | "transcript" | "logs" | "attempts" | "verdicts" | "rubrics" | "rubric";

// A scripted answer for one API path: a fixed reply, the real reply patched, a request that never
// answers (loading), or a dropped connection.
export type Mock = {
  path: RegExp;
  method?: string;
  query?: (q: URLSearchParams) => boolean;
  status?: number;
  body?: unknown;
  patch?: (json: any) => any; // eslint-disable-line @typescript-eslint/no-explicit-any
  hang?: true;
  abort?: true;
};

export type State = {
  name: string;
  type: PageType;
  path: string;
  ready: string | RegExp; // text on screen once the state is reached
  before?: string; // text on screen before `act` runs (default: ready)
  signedOut?: true;
  mocks?: Mock[];
  act?: (page: Page, phone: boolean) => Promise<void>;
};

const RUN = "/runs/superpipeline/brd_01/run_01";
const RUN_API = /^\/v1\/runs\/superpipeline\/brd_01\/run_01$/;
const SPAN = "00f067aa0ba902bc";
const fail = (status: number, code: string, message: string, extra: object = {}) => ({ status, body: { error: { code, message, ...extra } } });
const me = (status: number, code: string, message: string, extra: object = {}): Mock => ({ path: /^\/v1\/me$/, ...fail(status, code, message, extra) });

// Sixty runs, the last with values long enough to wrap: the inventory's "many" and "long" cases.
const LONG = "Reconcile the quarterly supplier invoices against the purchase orders and flag every line whose total differs by more than two percent";
function manyRuns() {
  const now = Date.now();
  const statuses = ["succeeded", "running", "waiting", "failed", "queued", "cancelled"];
  return Array.from({ length: 60 }, (_, i) => {
    const started = new Date(now - i * 3 * 3_600_000).toISOString();
    const last = i === 59;
    const pipeline = i % 7 !== 3;
    return {
      source: pipeline ? "superpipeline" : "agentpod", external_ref: pipeline ? `brd_01/run_${100 + i}` : `st_one/idx_${i}`, ref: `x:${i}`,
      scope: i === 5 ? null : { id: last ? "brd_" + "x".repeat(60) : "brd_01", name: last ? "A board with a very long name that keeps going and going" : "Press" },
      title: i === 6 ? null : last ? LONG : `Run number ${i}`,
      executor: i === 5 ? null : { id: null, name: last ? "prn_agent_0123456789abcdef0123456789abcdef0123456789abcdef" : "drafter" },
      status: statuses[i % 6], source_status: statuses[i % 6], started_at: i === 7 ? null : started, ended_at: i % 6 === 0 ? new Date(now - i * 3 * 3_600_000 + 90_000).toISOString() : null,
      reported_at: started, first_seen_at: started, updated_at: started,
      latest_verdict: i % 4 === 0 ? { id: `vrd_${i}`, judge: "prn_human01", judge_kind: "human", standard: "rubric:release@1",
        value: last ? { text: "A verdict whose free text runs well past forty characters" } : { decision: "pass" }, created_at: started } : null,
    };
  });
}
const counts = { queued: 10, running: 10, waiting: 10, succeeded: 10, failed: 10, cancelled: 10 };

// The run's verdict history, scripted so the list shows a superseded verdict and a revisable one
// (the fake mode has none until an e2e records one).
const at = (s: number) => new Date(Date.UTC(2026, 9, 18, 10, 0, s)).toISOString();
const hv = (id: string, over: object) => ({ id, kind: "review", subject_kind: "run", subject_ref: "superpipeline:brd_01/run_01", judge: "prn_human01", judge_kind: "human",
  standard: "rubric:release@1", value: { decision: "pass" }, comment: "", evidence_refs: [], supersedes: null, superseded_by: null, created_at: at(600), ...over });
const history: Mock = {
  path: /^\/v1\/verdicts$/, method: "GET", query: (q) => q.get("subject_kind") === "run", status: 200,
  body: { verdicts: [
    hv("vrd_01", { value: { decision: "fail" }, superseded_by: "vrd_02", created_at: at(500), comment: "Misses the upgrade steps." }),
    hv("vrd_02", { supersedes: "vrd_01", comment: "Clear and complete.", evidence_refs: [SPAN, { session_id: "acps_01", seq_from: 7, seq_to: 8 }] }),
  ] },
};

const openDrawer = async (page: Page) => {
  await page.getByRole("button", { name: "Record verdict" }).click();
  const d = page.getByRole("dialog", { name: "Record verdict" });
  await d.getByLabel("Rubric").selectOption("rubric:release@1");
  await d.getByRole("button", { name: "pass" }).click();
};

export const states: State[] = [
  // Edge pages (no shell).
  { name: "signed-out", type: "edge", path: "/", ready: "Sign in to browse runs and record verdicts.", signedOut: true },
  { name: "signed-out-deep", type: "edge", path: `${RUN}?tab=logs`, ready: "Sign in to browse runs and record verdicts.", signedOut: true },
  { name: "not-authorised", type: "edge", path: "/", ready: "Not authorised", mocks: [me(403, "not_authorised", "not authorised")] },
  { name: "database-down", type: "edge", path: "/", ready: "Can’t reach the database", mocks: [me(503, "store_unavailable", "the database is not answering")] },
  { name: "rate-limited", type: "edge", path: "/", ready: "Too many requests", mocks: [me(429, "rate_limited", "too many requests", { retry_after_seconds: 30 })] },
  { name: "principal-unresolved", type: "edge", path: "/", ready: "Can’t confirm who you are", mocks: [me(503, "principal_unresolved", "hub down")] },
  { name: "origin-mismatch", type: "edge", path: "/", ready: "Request refused", mocks: [me(403, "origin_mismatch", "origin mismatch")] },
  { name: "server-error", type: "edge", path: "/", ready: "Something went wrong", mocks: [me(500, "internal", "superwitness could not answer.")] },
  { name: "unreachable", type: "edge", path: "/", ready: "Something went wrong", mocks: [{ path: /^\/v1\/me$/, abort: true }] },
  { name: "app-loading", type: "edge", path: "/", ready: "Loading…", mocks: [{ path: /^\/v1\/me$/, hang: true }] },
  { name: "signed-out-lingering", type: "edge", path: "/", ready: "Signed out", before: "All runs", mocks: [{ path: /^\/auth\/logout$/, ...fail(503, "store_unavailable", "the database is not answering") }],
    act: async (page, phone) => {
      if (phone) await page.getByRole("button", { name: "Menu" }).click();
      else await page.locator(".sw-chip").click();
      await page.getByRole("menuitem", { name: "Sign out" }).click();
    } },
  { name: "sign-out-failed", type: "shell", path: "/", ready: "Sign out failed", before: "All runs", mocks: [{ path: /^\/auth\/logout$/, ...fail(500, "internal", "superwitness could not answer.") }],
    act: async (page, phone) => {
      if (phone) await page.getByRole("button", { name: "Menu" }).click();
      else await page.locator(".sw-chip").click();
      await page.getByRole("menuitem", { name: "Sign out" }).click();
    } },
  { name: "run-not-found", type: "edge", path: "/runs/superpipeline/brd_01/run_02", ready: "Run not found" },

  // The shell.
  { name: "page-not-found", type: "shell", path: "/nope", ready: "Page not found" },
  { name: "menu-open", type: "shell", path: "/", ready: "All runs",
    act: async (page, phone) => {
      if (phone) await page.getByRole("button", { name: "Menu" }).click();
      else await page.locator(".sw-chip").click();
    } },
  { name: "appearance-open", type: "shell", path: "/", ready: "All runs",
    act: async (page, phone) => {
      // On a phone the account menu is drawn open inside the Menu.
      if (phone) await page.getByRole("button", { name: "Menu" }).click();
      else await page.locator(".sw-chip").click();
      await page.getByRole("menuitem", { name: "Appearance" }).click();
    } },
  { name: "sources-degraded", type: "shell", path: "/", ready: "Sources degraded", before: "All runs",
    act: async (page, phone) => { if (phone) await page.getByRole("button", { name: "Menu" }).click(); }, // the dot is in the Menu
    mocks: [{ path: /^\/health$/, patch: (h) => ({ ...h, sources: { ...h.sources, traces: "timeout", agentpod: "unavailable" } }) }] },

  // Runs.
  { name: "runs-all", type: "runs", path: "/", ready: "Write the release note" },
  { name: "runs-needs-verdict", type: "runs", path: "/?needs_verdict=true", ready: "Needs verdict" },
  { name: "runs-failed", type: "runs", path: "/?status=failed", ready: "Failed" },
  { name: "runs-waiting", type: "runs", path: "/?status=waiting", ready: "Waiting" },
  { name: "runs-board", type: "runs", path: "/?scope=brd_01", ready: "Board brd_01" },
  { name: "runs-chip-selected", type: "runs", path: "/?status=running", ready: "Runs" },
  { name: "runs-two-statuses", type: "runs", path: "/?status=running&status=failed", ready: "Runs" },
  { name: "runs-empty", type: "runs", path: "/", ready: "No runs reported yet.",
    mocks: [{ path: /^\/v1\/runs$/, status: 200, body: { runs: [], next_cursor: null, counts: {} } }] },
  { name: "runs-empty-filtered", type: "runs", path: "/?status=cancelled", ready: "No runs in this view." },
  { name: "runs-many-long", type: "runs", path: "/", ready: "Run number 1",
    mocks: [{ path: /^\/v1\/runs$/, query: (q) => !q.has("cursor"), status: 200, body: { runs: manyRuns(), next_cursor: null, counts } }] },
  { name: "runs-more-error", type: "runs", path: "/", ready: "superwitness could not answer.",
    mocks: [
      { path: /^\/v1\/runs$/, query: (q) => q.has("cursor"), ...fail(500, "internal", "superwitness could not answer.") },
      { path: /^\/v1\/runs$/, query: (q) => !q.has("cursor"), patch: (j) => ({ ...j, next_cursor: "c1" }) },
    ] },
  { name: "runs-loading", type: "runs", path: "/", ready: "All runs", mocks: [{ path: /^\/v1\/runs$/, hang: true }] },
  { name: "runs-database-down", type: "runs", path: "/", ready: "Can’t reach the database", mocks: [{ path: /^\/v1\/runs$/, ...fail(503, "store_unavailable", "down") }] },
  { name: "runs-rate-limited", type: "runs", path: "/", ready: "Too many requests", mocks: [{ path: /^\/v1\/runs$/, ...fail(429, "rate_limited", "slow down", { retry_after_seconds: 30 }) }] },
  { name: "runs-server-error", type: "runs", path: "/", ready: "Something went wrong", mocks: [{ path: /^\/v1\/runs$/, ...fail(500, "internal", "superwitness could not answer.") }] },

  // The run page.
  { name: "run-trace", type: "run", path: RUN, ready: "dispatch" },
  { name: "run-trace-span", type: "run", path: `${RUN}?span=${SPAN}`, ready: "permission denied" },
  { name: "run-transcript", type: "transcript", path: `${RUN}?tab=transcript`, ready: "Write release-note.md" },
  { name: "run-transcript-seq", type: "transcript", path: `${RUN}?tab=transcript&attempt=attempt_01&seq=7-8`, ready: "Write release-note.md" },
  { name: "run-logs", type: "logs", path: `${RUN}?tab=logs`, ready: "Level" },
  { name: "run-errors", type: "run", path: `${RUN}?tab=errors`, ready: "Errors" },
  { name: "run-verdicts", type: "verdicts", path: `${RUN}?tab=verdicts`, ready: "Clear and complete.", mocks: [history] },
  { name: "run-attempts", type: "attempts", path: `${RUN}?tab=attempts`, ready: "attempt_01" },
  { name: "run-drawer", type: "verdicts", path: `${RUN}?tab=verdicts`, ready: "Save verdict", before: "Clear and complete.", mocks: [history], act: openDrawer },
  { name: "run-drawer-revise", type: "verdicts", path: `${RUN}?tab=verdicts`, ready: "Revise verdict", before: "Clear and complete.", mocks: [history],
    act: async (page) => { await page.getByRole("button", { name: "Revise" }).first().click(); } },
  { name: "run-drawer-refused", type: "verdicts", path: `${RUN}?tab=verdicts`, ready: "You've already revised this verdict", before: "Clear and complete.",
    mocks: [history, { path: /^\/v1\/verdicts$/, method: "POST", ...fail(409, "already_superseded", "already superseded") }],
    act: async (page) => {
      await openDrawer(page);
      await page.getByRole("dialog").getByRole("button", { name: "Save verdict" }).click();
    } },
  { name: "run-running", type: "run", path: RUN, ready: "in progress",
    mocks: [{ path: RUN_API, patch: (d) => ({ ...d, run: { ...d.run, state: "running", ended_at: null }, attempts: d.attempts.map((a: object) => ({ ...a, seq_to: null, state: "running" })) }) }] },
  { name: "run-unknown", type: "run", path: `${RUN}?tab=attempts`, ready: "unavailable",
    mocks: [{ path: RUN_API, patch: (d) => ({ run: { ref: d.run.ref, card: { title: "unknown" } }, attempts: [], errors: [], cost: {}, verdicts: [], sources: { ...d.sources, agentpod: "timeout", errors: "timeout" } }) }] },
  { name: "run-long-values", type: "attempts", path: `${RUN}?tab=attempts`, ready: "attempt_01",
    mocks: [{ path: RUN_API, patch: (d) => ({ ...d, run: { ...d.run, agent: "prn_agent_0123456789abcdef0123456789abcdef0123456789abcdef", card: { ...d.run.card, title: LONG } },
      attempts: d.attempts.map((a: { fingerprint?: object }) => ({ ...a, fingerprint: { ...a.fingerprint, digest: "sha256:9f2b6c0e1a7d4c3b8e5f60718293a4b5c6d7e8f9012345678abcdef012345678" } })) }) }] },
  { name: "run-spans-empty", type: "run", path: RUN, ready: "No spans.", mocks: [{ path: /\/run_01\/spans$/, status: 200, body: { spans: [] } }] },
  { name: "run-spans-error", type: "run", path: RUN, ready: "Try again", mocks: [{ path: /\/run_01\/spans$/, ...fail(503, "source_unavailable", "superwitness could not be reached") }] },
  { name: "run-transcript-forbidden", type: "transcript", path: `${RUN}?tab=transcript`, ready: "Transcripts aren't available to this account",
    mocks: [{ path: /\/run_01\/transcript/, ...fail(403, "transcripts_forbidden", "Transcripts aren't available to this account") }] },
  { name: "run-transcript-unavailable", type: "transcript", path: `${RUN}?tab=transcript`, ready: "Transcript unavailable, try again",
    mocks: [{ path: /\/run_01\/transcript/, ...fail(503, "source_unavailable", "no", { retryable: true }) }] },
  { name: "run-verdicts-failed", type: "verdicts", path: `${RUN}?tab=verdicts`, ready: "could not be read",
    mocks: [{ path: /^\/v1\/verdicts$/, method: "GET", ...fail(503, "store_unavailable", "superwitness could not be reached") }] },
  { name: "run-logs-empty", type: "logs", path: `${RUN}?tab=logs`, ready: "No log lines.", mocks: [{ path: /\/run_01\/logs$/, status: 200, body: { logs: [], next_cursor: null } }] },
  { name: "run-logs-error", type: "logs", path: `${RUN}?tab=logs`, ready: "Try again", mocks: [{ path: /\/run_01\/logs$/, ...fail(503, "source_unavailable", "superwitness could not be reached") }] },
  { name: "run-database-down", type: "run", path: RUN, ready: "Can’t reach the database", mocks: [{ path: RUN_API, ...fail(503, "store_unavailable", "down") }] },

  // Rubrics.
  { name: "rubrics", type: "rubrics", path: "/rubrics", ready: "Release note" },
  { name: "rubrics-empty", type: "rubrics", path: "/rubrics", ready: "No rubrics yet.", mocks: [{ path: /^\/v1\/rubrics$/, status: 200, body: { rubrics: [] } }] },
  { name: "rubrics-loading", type: "rubrics", path: "/rubrics", ready: "Loading…", mocks: [{ path: /^\/v1\/rubrics$/, hang: true }] },
  { name: "rubric", type: "rubric", path: "/rubrics/release/1", ready: "A decision: pass, fail." },
  { name: "rubric-not-found", type: "rubric", path: "/rubrics/release/9", ready: "Rubric not found" },
];

// One state per page type, swept in every vibe; the rest are swept on Daylight.
export const representative = ["runs-all", "run-trace-span", "run-transcript", "run-drawer", "run-logs", "run-attempts", "rubrics", "rubric", "signed-out", "database-down"];
