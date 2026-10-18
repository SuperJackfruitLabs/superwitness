// Visual check of the built app, for comparing by eye with the approved mockups.
//
//   cd web && npm run build && node e2e/shots.mjs [path] [--open]
//
// Serves ../internal/web/dist on a free port with stubs for /v1/me, /v1/scopes and /health (no Go
// server, no database), then screenshots `path` (default "/") at 1440x900 and 390x844 in each vibe,
// light and dark, chosen with the vk_appearance cookie. --open also opens the phone Menu and the
// account menu; --appearance opens the Appearance control too. /v1/runs answers the mockups' nine runs
// (--more: with a next cursor whose page stays in flight; --more-error: that page fails; --empty: none;
// --loading: never answers). Pictures go to test-results/shots/ (git-ignored). Not part of `make e2e`.
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
const variant = moreError ? "-moreerror" : more ? "-more" : empty ? "-empty" : loading ? "-loading" : "";
const name = (target === "/" ? "runs" : target.replace(/[^a-z0-9]+/gi, "-").replace(/^-|-$/g, "")) + variant;
for (const vibe of ["daylight", "paper", "studio", "quiet"]) {
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
