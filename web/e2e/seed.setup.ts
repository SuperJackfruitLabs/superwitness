import { execFileSync } from "node:child_process";
import { expect, test as setup } from "@playwright/test";

// A development reporter, bound to superpipeline by serve.sh's SW_RUN_SOURCES.
const REPORTER = "dev:prn_reporter01:service:runs:write";

setup("seed a rubric and three runs", async ({ request }) => {
  await expect.poll(async () => (await (await request.get("/health")).json()).sources?.verdicts, { timeout: 30_000 }).toBe("ok");
  try {
    execFileSync("../bin/superwitness", ["rubric-add", "-id", "release", "-version", "1", "-name", "Release note",
      "-scale", '{"kind":"decision","options":["pass","fail"]}', "-body-file", "e2e/rubric.md", "-created-by", "prn_human01"],
      { env: { ...process.env, SW_DATABASE_URL: process.env.E2E_DATABASE_URL }, stdio: "pipe" });
  } catch (e) {
    if (!String((e as { stderr?: Buffer }).stderr ?? "").includes("already exists")) throw e;
  }
  // Day headings depend on the clock, so the runs sit well inside today and yesterday (UTC,
  // the browser's time zone in playwright.config.ts) whatever the time of the run.
  const now = new Date();
  const midnight = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
  const today = Math.max(now.getTime() - 60_000, midnight + 1_000);
  const yesterdayNoon = midnight - 12 * 3_600_000;
  const iso = (ms: number) => new Date(ms).toISOString();
  const run = (ref: string, status: string, started: number, ended: number | null) => ({
    source: "superpipeline", external_ref: ref, scope: { id: "brd_01", name: "Press" },
    title: ref === "brd_01/run_01" ? "Write the release note" : `Run ${ref}`, executor: { name: "drafter" },
    status, source_status: status, started_at: iso(started), ended_at: ended === null ? null : iso(ended), reported_at: iso(now.getTime()),
  });
  const res = await request.post("/v1/runs", {
    headers: { Authorization: `Bearer ${REPORTER}` },
    data: { runs: [run("brd_01/run_01", "succeeded", today, today + 30_000), run("brd_01/run_02", "running", today, null),
      run("brd_01/run_03", "failed", yesterdayNoon, yesterdayNoon + 120_000)] },
  });
  expect(res.status(), await res.text()).toBe(200);
});
