// Visual check of the built app, for comparing by eye with the approved mockups.
//
//   cd web && npm run build && node e2e/shots.mjs [path] [--open]
//
// Serves ../internal/web/dist on a free port with stubs for /v1/me, /v1/scopes and /health (no Go
// server, no database), then screenshots `path` (default "/") at 1440x900 and 390x844 in each vibe,
// light and dark, chosen with the vk_appearance cookie. --open also opens the phone Menu and the
// account menu; --appearance opens the Appearance control too. Pictures go to test-results/shots/ (git-ignored). Not part of `make e2e`.
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
const name = target === "/" ? "runs" : target.replace(/[^a-z0-9]+/gi, "-").replace(/^-|-$/g, "");
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
      await page.screenshot({ path: file, fullPage: size === "phone" });
      console.log(file);
      await ctx.close();
    }
  }
}
await browser.close();
server.close();
