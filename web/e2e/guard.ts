import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const HUB = "http://127.0.0.1:8791";

// signIn goes through the stub hub as the development human. The stub signs in whoever it was last
// told to, and another spec may name someone else in between, so the identity is checked.
export async function signIn(page: Page, request: APIRequestContext) {
  for (let i = 0; ; i++) {
    await request.post(`${HUB}/__stub/sign-in-as`, { data: { sub: "hubuser_01", kind: "human", email: "human01@example.com" } });
    await page.goto("/");
    await page.getByRole("link", { name: "Sign in", exact: true }).click();
    const ok = await page.getByRole("heading", { name: "All runs" }).waitFor({ timeout: 5_000 }).then(() => true, () => false);
    if (ok) return;
    if (i >= 2) throw new Error("could not sign in as the development human");
    await page.context().clearCookies();
  }
}

// watch records what must never happen on a page: a console error (a refused resource, which the
// browser logs on every scripted /v1 failure, is not the app's error) and a CSP violation.
export function watch(page: Page): { problems: string[] } {
  const problems: string[] = [];
  page.on("console", (m) => {
    if (m.type() !== "error") return;
    if (m.text().startsWith("Failed to load resource")) return;
    problems.push(`console: ${m.text()}`);
  });
  page.on("pageerror", (e) => problems.push(`pageerror: ${e.message}`));
  void page.addInitScript(() => {
    document.addEventListener("securitypolicyviolation", (e) => {
      console.error(`CSP violation: ${e.violatedDirective} ${e.blockedURI}`);
    });
  });
  return { problems };
}

export async function noHorizontalScroll(page: Page) {
  const [sw, cw] = await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.clientWidth]);
  expect(sw, "the page scrolls sideways").toBeLessThanOrEqual(cw);
}
