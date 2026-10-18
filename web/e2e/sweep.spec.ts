import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { AUTH, watch } from "./guard";
import { type Mock, representative, states } from "./states";

// The sweep: every state on Daylight, one state per page type on Paper, Studio and Quiet, each in
// light and dark at 360, 390 and 1440 wide. A visit passes with no axe violation (WCAG 2 A and AA),
// no sideways page scroll, no console error or CSP violation, and, under 1024 wide, every control
// at least 44px each way (inline links inside running text excepted). Run alone with
// `npx playwright test --project=sweep` (make e2e runs it too).
const VIBES = ["daylight", "paper", "studio", "quiet"] as const;
const THEMES = ["light", "dark"] as const;
const WIDTHS = [360, 390, 1440] as const;

async function script(page: Page, mocks: Mock[]) {
  for (const m of mocks) {
    await page.route((url) => m.path.test(url.pathname), async (route) => {
      const req = route.request();
      const q = new URL(req.url()).searchParams;
      if ((m.method && req.method() !== m.method) || (m.query && !m.query(q))) return route.fallback();
      if (m.hang) return; // never answers: the loading state
      if (m.abort) return route.abort("connectionreset");
      if (m.patch) {
        const res = await route.fetch();
        return route.fulfill({ response: res, json: m.patch(await res.json()) });
      }
      return route.fulfill({ status: m.status ?? 200, json: m.body });
    });
  }
}

// smallTargets lists the visible controls under 44px either way. A link inside running text (an
// inline link whose parent has text of its own) is exempt, as WCAG 2.5.8 exempts it; a control
// that is visually hidden behind its label is measured by its label.
function smallTargets(page: Page) {
  return page.evaluate(() => {
    const sel = 'a[href], button, input:not([type="hidden"]), select, textarea, summary, [role="tab"], [role="menuitem"], [role="button"], [tabindex]:not([tabindex="-1"])';
    const out: string[] = [];
    for (const el of document.querySelectorAll<HTMLElement>(sel)) {
      const r = el.getBoundingClientRect();
      if (r.width === 0 && r.height === 0) continue;
      const cs = getComputedStyle(el);
      if (cs.visibility === "hidden" || el.closest("[inert], [aria-hidden='true']")) continue;
      if (el.tagName === "A" && cs.display === "inline") {
        const parent = el.parentElement;
        const own = (parent?.textContent ?? "").replace(el.textContent ?? "", "").trim();
        if (own.length > 0) continue;
      }
      const boxes = [r, ...[...((el as HTMLInputElement).labels ?? [])].map((l) => l.getBoundingClientRect())];
      if (boxes.some((b) => b.width >= 43.5 && b.height >= 43.5)) continue;
      out.push(`${el.tagName.toLowerCase()} "${(el.textContent || el.getAttribute("aria-label") || "").trim().slice(0, 40)}" ${Math.round(r.width)}x${Math.round(r.height)}`);
    }
    return out;
  });
}

test.describe.configure({ mode: "parallel" });

for (const vibe of VIBES) {
  for (const s of states.filter((x) => vibe === "daylight" || representative.includes(x.name))) {
    for (const theme of THEMES) {
      for (const width of WIDTHS) {
        test(`${s.name} ${vibe} ${theme} ${width}`, async ({ browser, baseURL }) => {
          const ctx = await browser.newContext({
            viewport: { width, height: width < 1024 ? 844 : 900 }, baseURL, timezoneId: "UTC", reducedMotion: "reduce",
            storageState: s.signedOut ? undefined : AUTH,
          });
          await ctx.addCookies([{ name: "vk_appearance", value: `${vibe}.${theme}.strong.0`, url: baseURL! }]);
          const page = await ctx.newPage();
          const { problems } = watch(page);
          await script(page, s.mocks ?? []);
          await page.goto(s.path);
          if (s.act) {
            await page.getByText(s.before ?? s.ready).filter({ visible: true }).first().waitFor();
            await s.act(page, width < 1024);
          }
          await expect(page.getByText(s.ready).filter({ visible: true }).first()).toBeVisible();
          await page.waitForTimeout(150);

          const axe = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
          const violations = axe.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(" | ")}`);
          expect.soft(violations, "axe").toEqual([]);
          const [sw, cw] = await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.clientWidth]);
          expect.soft(sw, "sideways scroll").toBeLessThanOrEqual(cw);
          if (width < 1024) expect.soft(await smallTargets(page), "touch targets").toEqual([]);
          expect.soft(problems, "console").toEqual([]);
          await ctx.close();
        });
      }
    }
  }
}
