import { expect, type Locator, type Page, test } from "@playwright/test";
import { AUTH, noHorizontalScroll, watch } from "./guard";

// Phone behaviours at 360 wide: the Menu opens in the page and closes on navigation, the tabs row
// shows the current tab, the span pane and the verdict drawer take the screen with their actions
// in reach, and long unbroken values never widen the page.
const RUN = "/runs/superpipeline/brd_01/run_01";
const SPAN = "00f067aa0ba902bc";
const AGENT = "prn_agent_0123456789abcdef0123456789abcdef0123456789abcdef"; // 60 characters
const DIGEST = "sha256:9f2b6c0e1a7d4c3b8e5f60718293a4b5c6d7e8f9012345678abcdef012345678"; // 71 characters

test.use({ viewport: { width: 360, height: 740 }, storageState: AUTH }); // the sweep's session (sweep.setup.ts)

async function inView(page: Page, el: Locator) {
  const b = (await el.boundingBox())!;
  const h = await page.evaluate(() => innerHeight);
  expect(b.y).toBeGreaterThanOrEqual(0);
  expect(b.y + b.height).toBeLessThanOrEqual(h + 1);
}

test("the Menu opens in the page, not over it, and closes on navigation", async ({ page }) => {
  const { problems } = watch(page);
  await page.goto("/");
  const nav = page.getByRole("navigation", { name: "Main" });
  const menu = page.getByRole("button", { name: "Menu" });
  await expect(nav).toBeHidden();
  await expect(menu).toHaveAttribute("aria-expanded", "false");
  const before = (await page.getByRole("heading", { level: 1, name: "All runs" }).boundingBox())!.y;
  await menu.click();
  await expect(menu).toHaveAttribute("aria-expanded", "true");
  await expect(nav).toBeVisible();
  // Inline: the menu sits under the top bar and pushes the page down.
  const bar = (await page.locator(".topbar").boundingBox())!;
  const n = (await nav.boundingBox())!;
  expect(n.y).toBeGreaterThanOrEqual(bar.y + bar.height - 1);
  expect((await page.getByRole("heading", { level: 1, name: "All runs" }).boundingBox())!.y).toBeGreaterThan(before + n.height / 2);
  await noHorizontalScroll(page);
  await nav.getByRole("link", { name: "Failed" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Failed" })).toBeVisible();
  await expect(nav).toBeHidden();
  await expect(menu).toHaveAttribute("aria-expanded", "false");
  expect(problems).toEqual([]);
});

test("the tabs row scrolls the current tab into view", async ({ page }) => {
  await page.goto("/");
  const strip = page.getByRole("tablist", { name: "Run" });
  for (const name of ["Attempts", "Verdicts", "Trace"]) {
    await page.goto(`${RUN}${name === "Trace" ? "" : `?tab=${name.toLowerCase()}`}`);
    const tab = strip.getByRole("tab", { name });
    await expect(tab).toHaveAttribute("aria-selected", "true");
    await expect(async () => {
      const [s, t] = [(await strip.boundingBox())!, (await tab.boundingBox())!];
      expect(t.x).toBeGreaterThanOrEqual(s.x - 1);
      expect(t.x + t.width).toBeLessThanOrEqual(s.x + s.width + 1);
    }).toPass();
  }
  // Choosing a tab by tapping keeps it in view too.
  await strip.getByRole("tab", { name: "Errors" }).click();
  await expect(page).toHaveURL(/tab=errors/);
  const [s, t] = [(await strip.boundingBox())!, (await strip.getByRole("tab", { name: "Errors" }).boundingBox())!];
  expect(t.x + t.width).toBeLessThanOrEqual(s.x + s.width + 1);
});

test("the span pane fills the screen and Cite stays in reach", async ({ page }) => {
  await page.goto("/");
  await page.goto(`${RUN}?span=${SPAN}`);
  const pane = page.getByRole("complementary", { name: "Span details" });
  await expect(pane).toContainText("permission denied");
  const box = (await pane.boundingBox())!;
  const [w, h] = await page.evaluate(() => [document.documentElement.clientWidth, innerHeight]);
  expect(box.x).toBe(0);
  expect(box.width).toBe(w);
  expect(box.y + box.height).toBeGreaterThanOrEqual(h - 1);
  const cite = pane.getByRole("button", { name: "Cite" });
  await inView(page, cite);
  await pane.evaluate((el) => {
    const scroller = [el, ...el.querySelectorAll<HTMLElement>("*")].find((e) => e.scrollHeight > e.clientHeight + 1 && getComputedStyle(e).overflowY !== "visible");
    scroller?.scrollTo(0, scroller.scrollHeight / 2);
  });
  await inView(page, cite);
  await noHorizontalScroll(page);
});

test("the verdict drawer fills the screen and Save stays in reach", async ({ page }) => {
  await page.goto("/");
  await page.goto(RUN);
  await page.getByRole("button", { name: "Record verdict" }).click();
  const drawer = page.getByRole("dialog", { name: "Record verdict" });
  await drawer.getByLabel("Rubric").selectOption("rubric:release@1");
  await drawer.getByLabel("Comment (optional)").fill("A comment\n".repeat(40));
  const box = (await drawer.boundingBox())!;
  const [w, h] = await page.evaluate(() => [document.documentElement.clientWidth, innerHeight]);
  expect(box.x).toBe(0);
  expect(box.width).toBe(w);
  expect(box.y + box.height).toBeGreaterThanOrEqual(h - 1);
  for (const name of ["Save verdict", "Cancel"]) await inView(page, drawer.getByRole("button", { name }));
  await drawer.getByRole("button", { name: "Cancel" }).click();
  await expect(drawer).toBeHidden();
});

test("a 60-character agent and a 71-character digest do not widen the page", async ({ page }) => {
  await page.goto("/");
  await page.route((u) => u.pathname === `/v1${RUN}`, async (route) => {
    const res = await route.fetch();
    const d = await res.json();
    d.run.agent = AGENT;
    d.attempts = d.attempts.map((a: { fingerprint?: object }) => ({ ...a, fingerprint: { ...a.fingerprint, digest: DIGEST } }));
    await route.fulfill({ response: res, json: d });
  });
  await page.goto(`${RUN}?tab=attempts`);
  await expect(page.getByText(AGENT)).toBeVisible();
  await expect(page.getByText(DIGEST).first()).toBeVisible();
  await noHorizontalScroll(page);
  await page.goto(RUN);
  await expect(page.getByText(AGENT)).toBeVisible();
  await noHorizontalScroll(page);
});
