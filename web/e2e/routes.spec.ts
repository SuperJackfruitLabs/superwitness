import { expect, test } from "@playwright/test";
import { AUTH, watch } from "./guard";

// The URL table, through the real binary: each URL lands on its page (anything else is the in-shell
// "Page not found"), and a reload keeps the state the query string holds. No URL logs an error or
// trips the Content Security Policy.
test.use({ storageState: AUTH }); // the sweep's session (sweep.setup.ts)

const RUN = "/runs/superpipeline/brd_01/run_01";
const SPAN = "00f067aa0ba902bc";

const table: { url: string; heading: string; notFound?: boolean }[] = [
  { url: "/", heading: "All runs" },
  { url: RUN, heading: "Write the release note" },
  { url: `${RUN}/`, heading: "Write the release note" },
  { url: "/rubrics", heading: "Rubrics" },
  { url: "/rubrics/", heading: "Rubrics" },
  { url: "/rubrics/release/1", heading: "Release note" },
  { url: "/rubrics/release/0", heading: "Page not found", notFound: true },
  { url: "/runs/superpipeline/brd_01", heading: "Page not found", notFound: true },
  { url: "/nope", heading: "Page not found", notFound: true },
];

test("every URL in the table lands on its page, and on a reload", async ({ page }) => {
  const { problems } = watch(page);
  await page.goto("/");
  for (const row of table) {
    await test.step(row.url, async () => {
      await page.goto(row.url);
      for (const pass of ["load", "reload"]) {
        if (pass === "reload") await page.reload();
        await expect(page.getByRole("heading", { level: 1, name: row.heading, exact: true }), `${row.url} ${pass}`).toBeVisible();
        await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible(); // in the shell, not found included
        if (row.notFound) await expect(page.getByRole("link", { name: "All runs" }).last()).toBeVisible();
        expect(new URL(page.url()).pathname).toBe(row.url);
      }
    });
  }
  expect(problems).toEqual([]);
});

test("a reload keeps the state the query string holds", async ({ page }) => {
  const { problems } = watch(page);
  await page.goto("/");
  const keep = async (url: string, check: () => Promise<void>) => {
    await test.step(url, async () => {
      await page.goto(url);
      await check();
      await page.reload();
      await check();
      expect(page.url().slice(page.url().indexOf("/", 8))).toBe(url);
    });
  };

  await keep("/?needs_verdict=true", async () => {
    await expect(page.getByRole("heading", { level: 1, name: "Needs verdict" })).toBeVisible();
  });
  await keep("/?status=failed", async () => {
    await expect(page.getByRole("heading", { level: 1, name: "Failed" })).toBeVisible();
    await expect(page.getByRole("list", { name: "Runs by status" }).locator('[aria-current="true"]')).toContainText("failed");
  });
  await keep("/?scope=brd_01", async () => {
    await expect(page.getByRole("heading", { level: 1, name: "Board brd_01" })).toBeVisible();
  });
  await keep("/?source=superpipeline&status=running&status=failed&executor=drafter&since=2026-01-01T00:00:00Z&until=2099-01-01T00:00:00Z", async () => {
    await expect(page.getByRole("heading", { level: 1, name: "Runs", exact: true })).toBeVisible();
    await expect(page.getByRole("list", { name: "Runs by status" }).locator('[aria-current="true"]')).toHaveCount(0);
  });
  await keep(`${RUN}?tab=logs`, async () => {
    await expect(page.getByRole("tab", { name: "Logs", selected: true })).toBeVisible();
  });
  await keep(`${RUN}?span=${SPAN}`, async () => {
    await expect(page.getByRole("complementary", { name: "Span details" })).toContainText("permission denied");
  });
  await keep(`${RUN}?tab=transcript&attempt=attempt_01&seq=7-8`, async () => {
    await expect(page.getByRole("tab", { name: "Transcript", selected: true })).toBeVisible();
    await expect(page.locator('.transcript > li[aria-current="true"]')).toContainText("Write release-note.md");
  });
  await keep(`${RUN}?tab=attempts`, async () => {
    await expect(page.getByRole("tab", { name: "Attempts", selected: true })).toBeVisible();
    await expect(page.getByText("attempt_01").first()).toBeVisible();
  });
  expect(problems).toEqual([]);
});
