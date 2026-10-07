import { expect, type APIRequestContext, test } from "@playwright/test";

const HUB = "http://127.0.0.1:8791";

async function signInAs(request: APIRequestContext, sub: string, email = "") {
  await request.post(`${HUB}/__stub/sign-in-as`, { data: { sub, kind: "human", email } });
}

test("someone the allowlist does not name is refused", async ({ page, request }) => {
  await signInAs(request, "hubuser_7f3a"); // the development set's prn_human02
  await page.goto("/");
  await page.getByRole("link", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Not authorised" })).toBeVisible();
});

test("sign in, browse, record a verdict and revise it", async ({ page, request }) => {
  await signInAs(request, "hubuser_01", "human01@example.com");

  await test.step("sign in", async () => {
    await page.goto("/");
    await page.getByRole("link", { name: "Sign in", exact: true }).click();
    await expect(page.getByRole("heading", { name: "All runs" })).toBeVisible();
    await expect(page.getByText("human01@example.com")).toBeVisible();
  });

  await test.step("the list groups runs by day", async () => {
    await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Yesterday" })).toBeVisible();
    await page.getByRole("link", { name: "Needs verdict" }).click();
    await expect(page.getByRole("heading", { name: "Needs verdict" })).toBeVisible();
    await expect(page.getByRole("link", { name: /Write the release note/ })).toBeVisible();
  });

  await test.step("the run view opens on its trace", async () => {
    await page.getByRole("link", { name: /Write the release note/ }).click();
    await expect(page).toHaveURL(/\/runs\/superpipeline\/brd_01\/run_01$/);
    await expect(page.getByRole("tab", { name: "Trace", selected: true })).toBeVisible();
    await page.getByRole("checkbox", { name: "Cite dispatch as evidence" }).check();
    await page.getByRole("tab", { name: "Logs" }).click();
    await expect(page).toHaveURL(/tab=logs/);
  });

  await test.step("record a verdict", async () => {
    await page.getByRole("button", { name: "Record verdict" }).click();
    const drawer = page.getByRole("dialog", { name: "Record verdict" });
    await drawer.getByLabel("Rubric").selectOption("rubric:release@1");
    await drawer.getByRole("button", { name: "pass" }).click();
    await expect(drawer.getByText("1 span ticked in the Trace tab")).toBeVisible();
    // The first save reaches the server, but its answer is lost on the way back: the retry below
    // carries the same idempotency key, made when the drawer opened, so it records nothing new.
    await page.route("**/v1/verdicts", async (route) => {
      await route.fetch();
      await route.abort("connectionreset");
    }, { times: 1 });
    await drawer.getByRole("button", { name: "Save verdict" }).click();
    await expect(drawer.getByRole("alert")).toBeVisible();
    await drawer.getByRole("button", { name: "Save verdict" }).dblclick(); // one verdict, not two
    await expect(drawer).toBeHidden();
    await page.getByRole("tab", { name: "Verdicts" }).click();
    await expect(page.locator(".verdict").filter({ hasText: "rubric:release@1" })).toHaveCount(1);
    await expect(page.locator(".verdict").filter({ hasText: "rubric:release@1" })).toContainText("pass");
  });

  await test.step("the run leaves Needs verdict", async () => {
    await page.getByRole("link", { name: "Needs verdict" }).click();
    await expect(page.getByRole("heading", { name: "Needs verdict" })).toBeVisible();
    await expect(page.getByRole("link", { name: /Write the release note/ })).toHaveCount(0);
  });

  await test.step("revise it", async () => {
    await page.goto("/runs/superpipeline/brd_01/run_01?tab=verdicts");
    await page.getByRole("button", { name: "Revise" }).click();
    const drawer = page.getByRole("dialog", { name: "Revise verdict" });
    await expect(drawer.getByText("1 cited: 1 from the verdict being revised")).toBeVisible(); // the cited span carries over
    await drawer.getByRole("button", { name: "fail" }).click();
    await drawer.getByRole("button", { name: "Save verdict" }).click();
    await expect(drawer).toBeHidden();
    await expect(page.locator(".verdict.superseded")).toContainText("pass");
    await expect(page.getByRole("link", { name: /superseded by vrd_/ })).toBeVisible();
    await expect(page.getByRole("button", { name: "Revise" })).toHaveCount(1);
  });

  await test.step("the card shows the latest verdict", async () => {
    await page.getByRole("link", { name: "All", exact: true }).click();
    await expect(page.getByRole("link", { name: /Write the release note/ })).toContainText("verdict: fail");
  });

  await test.step("sign out", async () => {
    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("link", { name: "Sign in", exact: true })).toBeVisible();
    await expect(page.getByRole("alert")).toHaveCount(0);
  });
});

test("on a phone the sidebar folds into a menu", async ({ page, request }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await signInAs(request, "hubuser_01", "human01@example.com");
  await page.goto("/");
  await page.getByRole("link", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("heading", { name: "All runs" })).toBeVisible();
  const nav = page.getByRole("navigation", { name: "Main" });
  await expect(nav).toBeHidden();
  await page.getByRole("button", { name: "Menu" }).click();
  await nav.getByRole("link", { name: "Rubrics" }).click();
  await expect(page.getByRole("heading", { name: "Rubrics" })).toBeVisible();
  await expect(nav).toBeHidden();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  // A short page: the sticky bar keeps its own height rather than sharing out the spare screen.
  expect((await page.locator(".topbar").boundingBox())!.height).toBeLessThanOrEqual(72);

  // The last run tab, selected, is scrolled into view, and every control is a 44px touch target.
  await page.goto("/runs/superpipeline/brd_01/run_01?tab=attempts");
  const strip = page.getByRole("tablist", { name: "Run" });
  const tab = strip.getByRole("tab", { name: "Attempts" });
  await expect(tab).toHaveAttribute("aria-selected", "true");
  const [s, t] = [(await strip.boundingBox())!, (await tab.boundingBox())!];
  expect(t.x).toBeGreaterThanOrEqual(s.x);
  expect(t.x + t.width).toBeLessThanOrEqual(s.x + s.width + 1);
  await expect(strip).toHaveAttribute("data-more", /left/);
  await expect(page.getByText("attempt_01")).toBeVisible();
  const small = await page.evaluate(() =>
    [...document.querySelectorAll<HTMLElement>("button, [role=tab], .chips a, .cite")]
      .filter((e) => e.offsetParent !== null && e.getBoundingClientRect().height < 44)
      .map((e) => e.textContent || e.outerHTML.slice(0, 60)),
  );
  expect(small).toEqual([]);
  await page.goto("/runs/superpipeline/brd_01/run_01");
  await page.locator(".cite").first().waitFor();
  const ticks = await page.locator(".cite").evaluateAll((es) => es.map((e) => e.getBoundingClientRect()).map((r) => Math.min(r.width, r.height)));
  expect(ticks.length).toBeGreaterThan(0);
  expect(Math.min(...ticks)).toBeGreaterThanOrEqual(44);
});
