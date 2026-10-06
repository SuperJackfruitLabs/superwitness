import { expect, type APIRequestContext, type Page, test } from "@playwright/test";

const HUB = "http://127.0.0.1:8791";
const RUN = "/runs/superpipeline/brd_01/run_01";
const API = "/v1/runs/superpipeline/brd_01/run_01";
const FAILED_CALL = "00f067aa0ba902bc"; // the development set's failing tool_call span, seq 7..8

async function signIn(page: Page, request: APIRequestContext) {
  await request.post(`${HUB}/__stub/sign-in-as`, { data: { sub: "hubuser_01", kind: "human", email: "human01@example.com" } });
  await page.goto("/");
  await page.getByRole("link", { name: "Sign in with AgentPod" }).click();
  await expect(page.getByRole("heading", { name: "All runs" })).toBeVisible();
}

test("a span's request and response, its transcript card, and a cited step", async ({ page, request }) => {
  await signIn(page, request);
  await page.goto(RUN);
  const pane = page.getByRole("complementary", { name: "Span details" });

  await test.step("the failing tool call's span shows what was sent and what came back", async () => {
    await page.locator(".waterfall li").filter({ hasText: "tool_call" }).nth(1).getByRole("button", { name: "tool_call" }).click();
    await expect(page).toHaveURL(new RegExp(`\\?span=${FAILED_CALL}$`));
    await expect(pane).toContainText("ERROR");
    await expect(pane).toContainText('"path": "release-note.md"');
    await expect(pane).toContainText("permission denied: release-note.md is read-only");
    await expect(pane).toContainText("0 values redacted");
  });

  await test.step("Open in transcript lands on the same card, and span ↗ comes back", async () => {
    await pane.getByRole("link", { name: "Open in transcript ↗" }).click();
    await expect(page).toHaveURL(/\?tab=transcript&attempt=attempt_01&seq=7-8$/);
    const card = page.locator('.transcript > li[aria-current="true"]');
    await expect(card).toHaveCount(1);
    await expect(card).toContainText("Write release-note.md");
    await expect(card).toHaveClass(/failed/);
    await expect(card.locator("details")).toHaveAttribute("open", "");
    await card.getByRole("link", { name: "span ↗" }).click();
    await expect(page).toHaveURL(new RegExp(`\\?span=${FAILED_CALL}$`));
    await expect(pane).toBeVisible();
  });

  await test.step("the prompt shows the hub's redaction, never the secret", async () => {
    await page.getByRole("tab", { name: "Transcript" }).click();
    const prompt = page.locator('.transcript > li[data-kind="prompt"]');
    await expect(prompt).toContainText("[redacted:anthropic-key]");
    await expect(prompt).toContainText("[redacted:authorization]");
    await expect(prompt).toContainText("2 redacted");
  });

  await test.step("show the whole of a cut field", async () => {
    const read = page.locator(".transcript > li").filter({ hasText: "Read CHANGELOG.md" });
    await read.locator("summary").click();
    await expect(read).not.toContainText("- change 1199:");
    await read.getByRole("button", { name: "Show full" }).click();
    await expect(read).toContainText("- change 1199: a line of the development changelog");
  });

  await test.step("cite the prompt in a verdict on the attempt", async () => {
    await page.locator('.transcript > li[data-kind="prompt"]').getByRole("button", { name: "Cite" }).click();
    await page.getByRole("button", { name: "Record verdict" }).click();
    const drawer = page.getByRole("dialog", { name: "Record verdict" });
    await drawer.getByLabel("Subject").selectOption("attempt|attempt_01");
    await drawer.getByLabel("Rubric").selectOption("rubric:release@1");
    await drawer.getByRole("button", { name: "pass" }).click();
    await expect(drawer.getByText("1 cited: 1 step from the transcript")).toBeVisible();
    await drawer.getByRole("button", { name: "Save verdict" }).click();
    await expect(drawer).toBeHidden();
  });

  await test.step("the verdict links back to the cited range", async () => {
    await page.getByRole("tab", { name: "Verdicts" }).click();
    await page.locator(".verdict").filter({ hasText: "attempt_01" }).getByRole("link", { name: "transcript 1–1 ↗" }).click();
    await expect(page).toHaveURL(/\?tab=transcript&attempt=attempt_01&seq=1$/);
    await expect(page.locator('.transcript > li[aria-current="true"]')).toHaveAttribute("data-kind", "prompt");
  });
});

test("on a phone the span pane fills the screen", async ({ page, request }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await signIn(page, request);
  await page.goto(`${RUN}?span=${FAILED_CALL}`);
  const pane = page.getByRole("complementary", { name: "Span details" });
  await expect(pane).toContainText("permission denied");
  const box = await pane.boundingBox();
  expect(box?.x).toBe(0);
  expect(box?.width).toBe(await page.evaluate(() => document.documentElement.clientWidth));
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

test("a token needs transcripts:read, and the answer is never cached", async ({ request }) => {
  const refused = await request.get(`${API}/transcript`, { headers: { Authorization: "Bearer dev:prn_grader01:agent:evidence:read" } });
  expect(refused.status()).toBe(403);
  expect((await refused.json()).error.code).toBe("transcripts_forbidden");
  const ok = await request.get(`${API}/transcript`, { headers: { Authorization: "Bearer dev:prn_grader01:agent:transcripts:read" } });
  expect(ok.status()).toBe(200);
  expect(ok.headers()["cache-control"]).toBe("no-store");
  expect(await ok.text()).toContain("[redacted:anthropic-key]");
});
