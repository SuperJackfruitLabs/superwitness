// Visual check of the built app, for comparing by eye with the approved mockups.
//
//   cd web && npm run build && node e2e/shots.mjs [path] [--open]
//
// Serves ../internal/web/dist on a free port with stubs for /v1/me, /v1/scopes and /health (no Go
// server, no database), then screenshots `path` (default "/") at 1440x900 and 390x844 in each vibe,
// light and dark, chosen with the vk_appearance cookie. --open also opens the phone Menu and the
// account menu; --appearance opens the Appearance control too. /v1/runs answers the mockups' nine runs
// (--more: with a next cursor whose page stays in flight; --more-error: that page fails; --empty: none;
// --loading: never answers). The run page /runs/superpipeline/brd_01/run_01 answers FACTS.md's run
// "Write the release note" with its 7 spans, logs and the failed tool_call's transcript (open it with
// ?span=00f067aa0ba902bc); --running: the run is still running; --unknown: every figure unknown and
// the agentpod source timed out; --long-agent: a 60-character agent principal; --spans-empty,
// --spans-loading, --spans-error, --spans-more: the trace's own states. Pictures go to
// test-results/shots/ (git-ignored). Not part of `make e2e`.
import { createServer } from "node:http";
import { readFile, mkdir } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "@playwright/test";

const here = fileURLToPath(new URL(".", import.meta.url));
const dist = join(here, "../../internal/web/dist");
const out = join(here, "../test-results/shots");
const args = process.argv.slice(2);
const target = args.find((a) => a.startsWith("/")) ?? "/";
const panel = args.includes("--appearance");
const opened = panel || args.includes("--open");
const moreError = args.includes("--more-error");
const more = moreError || args.includes("--more");
const empty = args.includes("--empty");
const loading = args.includes("--loading");
const running = args.includes("--running");
const unknown = args.includes("--unknown");
const longAgent = args.includes("--long-agent");
const spansState = ["empty", "loading", "error", "more"].find((k) => args.includes(`--spans-${k}`));

// The mockups' runs: Today, Yesterday and Tue, 13 Oct 2026, the six statuses, one source with no
// run page, one very long title.
const H = 3_600_000;
const t = (ms) => new Date(Date.now() - ms).toISOString();
const day13 = (h, m) => new Date(2026, 9, 13, h, m).toISOString();
const press = { id: "brd_press", name: "Press" };
const station = { id: "st_one", name: "Station one" };
const ran = (n, title, status, scope, executor, started, secs, value) => ({
  source: n === 4 ? "agentpod" : "superpipeline",
  external_ref: n === 4 ? "st_one/idx_204" : `brd_press/run_0${n}`,
  ref: `x:${n}`, scope, title, executor: { id: null, name: executor }, status, source_status: status,
  started_at: started, ended_at: secs === null ? null : new Date(Date.parse(started) + secs * 1000).toISOString(),
  reported_at: started, first_seen_at: started, updated_at: started,
  latest_verdict: value ? { id: `vrd_${n}`, judge: "prn_human01", judge_kind: "human", standard: "rubric:release@1", value, created_at: started } : null,
});
const yesterday = (h) => { const d = new Date(); d.setDate(d.getDate() - 1); d.setHours(h, 0, 0, 0); return d.toISOString(); };
const runs = [
  ran(1, "Write the release note", "succeeded", press, "drafter", t(0.2 * H), 30, { decision: "pass" }),
  ran(2, "Summarise the changelog for 0.4", "running", press, "drafter", t(5 * 60_000), null, null),
  ran(3, "Triage the overnight alert queue", "waiting", station, "kai", t(2 * H), null, null),
  ran(4, "Nightly index rebuild", "succeeded", station, "indexer", t(3 * H), 3900, { score: 0.8 }),
  ran(5, "Rewrite the onboarding email", "failed", press, "drafter", yesterday(16), 120, { decision: "fail" }),
  ran(6, "Compare two draft headlines for the launch post", "succeeded", press, "drafter", yesterday(14), 252, { label: "formal" }),
  ran(7, "Reconcile the quarterly supplier invoices against the purchase orders and flag every line whose total differs by more than two percent", "failed", press, "drafter", yesterday(10), 760, null),
  ran(8, "Archive the 2025 press kit", "succeeded", press, "drafter", day13(15, 0), 18, { decision: "pass" }),
  ran(9, "Draft the weekly digest", "succeeded", press, "drafter", day13(9, 30), 95, { decision: "pass" }),
];
const counts = { queued: 0, running: 1, waiting: 1, succeeded: 5, failed: 2, cancelled: 0 };

const stubs = {
  "/v1/me": { principal: "prn_human01", kind: "human", email: "human01@example.com", via: "session" },
  "/v1/scopes": {
    scopes: [
      { source: "superpipeline", id: "brd_press", name: "Press", runs: 7 },
      { source: "agentpod", id: "st_one", name: "Station one", runs: 2 },
    ],
  },
  "/health": { status: "ok", version: "dev", sources: { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", transcripts: "ok" } },
};
// The run page: FACTS.md's run_01, seven spans over five minutes this morning.
const T0 = (() => { const d = new Date(); d.setHours(10, 0, 0, 0); return d.getTime(); })();
const at = (s) => new Date(T0 + s * 1000).toISOString();
const sources = { superpipeline: "ok", agentpod: "ok", traces: "ok", logs: "ok", errors: "ok", verdicts: "ok" };
const runDoc = {
  run: { ref: "superpipeline:brd_01/run_01", agent: longAgent ? "prn_agent_0123456789abcdef0123456789abcdef0123456789abcdef" : "prn_agent01", state: running ? "running" : "completed",
    started_at: at(0), ended_at: running ? null : at(300), card: { title: "Write the release note", stage: "draft" } },
  attempts: [{ id: "attempt_01", station: "stn_01", session_id: "acps_01", seq_from: 1, seq_to: running ? null : 9, state: running ? "running" : "completed" }],
  errors: running ? [] : [{ at: at(182.4), service: "agentpod-hub", message: "bridge heartbeat failed: 502 from superpipeline" }],
  cost: { status: "unreported" },
  verdicts: [],
  sources,
};
const unknownDoc = { run: { ref: "superpipeline:brd_01/run_01", card: { title: "unknown" } }, attempts: [], errors: [], cost: {}, verdicts: [], sources: { ...sources, agentpod: "timeout", errors: "timeout" } };
const TR = "4bf92f3577b34da6a3ce929d0e0e4736";
const hub = (id, parent, name, start, ms, attrs = {}) => ({ trace_id: TR, span_id: id, parent_span_id: parent, name, service: "agentpod-hub", start: at(start), duration_ms: ms,
  attributes: { "attempt.id": "attempt_01", "station.id": "stn_01", "run.id": "run_01", ...attrs } });
const spans = [
  hub("7a1c9e04b2d35f60", undefined, "dispatch", 0, 300000, { "board.id": "brd_01", "card.id": "crd_01" }),
  hub("1f2e3d4c5b6a7980", "7a1c9e04b2d35f60", "attempt", 2, 296000, { "harness.name": "hermes" }),
  hub("2a3b4c5d6e7f8091", "1f2e3d4c5b6a7980", "turn", 4.5, 290000, { "acp.seq_from": "1", "acp.seq_to": "9" }),
  hub("3b4c5d6e7f809102", "2a3b4c5d6e7f8091", "tool_call", 6, 1200, { "acp.seq_from": "3", "acp.seq_to": "4", "tool.kind": "read", "tool.status": "completed" }),
  hub("4c5d6e7f80910213", "2a3b4c5d6e7f8091", "permission", 114, 4000, { "acp.seq_from": "5", "acp.seq_to": "6", "permission.auto": "false" }),
  hub("00f067aa0ba902bc", "2a3b4c5d6e7f8091", "tool_call", 182, 800, { "acp.seq_from": "7", "acp.seq_to": "8", "tool.kind": "edit", "tool.status": "failed", "otel.status_code": "ERROR" }),
  { trace_id: "9c8b7a6f5e4d3c2b1a0f9e8d7c6b5a49", span_id: "5d6e7f8091021324", name: "POST /v1/boards/:id/runs/:runId/heartbeat", service: "superpipeline-api", start: at(288), duration_ms: 12, attributes: { "http.status_code": "200" } },
];
const logs = [
  { at: at(2.2), service: "agentpod-hub", level: "info", message: "attempt opened", trace_id: TR },
  { at: at(182.4), service: "agentpod-hub", level: "error", message: "bridge heartbeat failed: 502 from superpipeline", trace_id: TR },
];
const failedCall = { kind: "tool_call", id: "tc_2", seq_from: 7, seq_to: 8, title: "Write release-note.md", tool_kind: "edit", status: "failed", redactions: 0,
  input: { path: "release-note.md" }, output: { content: [{ type: "text", text: "permission denied: release-note.md is read-only" }], raw: null } };
const runAPI = "/v1/runs/superpipeline/brd_01/run_01";
function runStub(path, q) {
  if (path === runAPI) return [200, unknown ? unknownDoc : runDoc];
  if (path === `${runAPI}/spans`) {
    if (spansState === "loading") return null;
    if (spansState === "error") return [503, { error: { code: "source_unavailable", message: "superwitness could not be reached" } }];
    if (spansState === "empty") return [200, { spans: [] }];
    if (spansState === "more") return q.has("cursor") ? null : [200, { spans, next_cursor: "c2" }];
    return [200, { spans }];
  }
  if (path === `${runAPI}/logs`) return [200, { logs, next_cursor: null }];
  if (path.startsWith(`${runAPI}/transcript`)) return [200, { attempt_id: "attempt_01", session_id: "acps_01", seq_from: 7, seq_to: 8, items: [failedCall], next_cursor: null, redactions: 0, truncated_fields: 0 }];
  if (path === "/v1/verdicts") return [200, { verdicts: [] }];
  if (path === "/v1/rubrics") return [200, { rubrics: [] }];
  return undefined;
}

const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".woff2": "font/woff2", ".json": "application/json" };

const server = createServer(async (req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/v1/runs") {
    if (loading) return;
    const json = (status, body) => {
      res.writeHead(status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(body));
    };
    if (!new URL(req.url, "http://x").searchParams.has("cursor"))
      return json(200, { runs: empty ? [] : runs, next_cursor: more ? "c1" : null, counts: empty ? { ...counts, running: 0, waiting: 0, succeeded: 0, failed: 0 } : counts });
    // The next page, asked for as soon as the end scrolls near.
    if (moreError) return json(500, { error: { code: "internal", message: "superwitness could not answer." } });
    return; // --more: stays in flight for the picture
  }
  const answer = runStub(path, new URL(req.url, "http://x").searchParams);
  if (answer === null) return; // stays in flight for the picture
  if (answer) {
    res.writeHead(answer[0], { "Content-Type": "application/json" });
    return res.end(JSON.stringify(answer[1]));
  }
  if (stubs[path]) {
    res.writeHead(200, { "Content-Type": "application/json" });
    return res.end(JSON.stringify(stubs[path]));
  }
  if (path.startsWith("/v1/")) {
    res.writeHead(404, { "Content-Type": "application/json" });
    return res.end(JSON.stringify({ error: { code: "not_found", message: "stub" } }));
  }
  const file = normalize(join(dist, path));
  try {
    if (!file.startsWith(dist) || path.endsWith("/")) throw new Error("index");
    const body = await readFile(file);
    res.writeHead(200, { "Content-Type": types[extname(file)] ?? "application/octet-stream" });
    res.end(body);
  } catch {
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(await readFile(join(dist, "index.html")));
  }
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const base = `http://127.0.0.1:${server.address().port}`;

await mkdir(out, { recursive: true });
const browser = await chromium.launch();
const variant = (moreError ? "-moreerror" : more ? "-more" : empty ? "-empty" : loading ? "-loading" : "") + (running ? "-running" : "") + (unknown ? "-unknown" : "") +
  (longAgent ? "-longagent" : "") + (spansState ? `-spans${spansState}` : "");
const only = args.find((a) => a.startsWith("--only="))?.slice(7).split(",");
const name = (target === "/" ? "runs" : target.replace(/[^a-z0-9]+/gi, "-").replace(/^-|-$/g, "")) + variant;
for (const vibe of ["daylight", "paper", "studio", "quiet"].filter((v) => !only || only.includes(v))) {
  for (const theme of ["light", "dark"]) {
    for (const [w, h, size] of [[1440, 900, "desk"], [390, 844, "phone"]]) {
      const ctx = await browser.newContext({ viewport: { width: w, height: h } });
      await ctx.addCookies([{ name: "vk_appearance", value: `${vibe}.${theme}.strong.0`, url: base }]);
      const page = await ctx.newPage();
      await page.goto(base + target);
      await page.locator("#nav").waitFor({ state: "attached" });
      await page.waitForTimeout(300);
      if (opened) {
        if (size === "phone") await page.getByRole("button", { name: "Menu" }).click();
        else await page.locator(".sw-chip").click();
        if (panel) await page.locator(".sw-mi", { hasText: "Appearance" }).click();
        await page.waitForTimeout(150);
      }
      const file = join(out, `${name}-${vibe}-${theme}-${size}${panel ? "-appearance" : opened ? "-open" : ""}.png`);
      await page.screenshot({ path: file, fullPage: true });
      console.log(file);
      await ctx.close();
    }
  }
}
await browser.close();
server.close();
