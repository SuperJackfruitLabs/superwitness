import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/svelte";
import Shell from "./Shell.svelte";
import { appearance } from "../appearance.svelte";
import type { Me } from "../types";
import { createRawSnippet } from "svelte";

const me: Me = { principal: "prn_human01", kind: "human", email: "a@example.com", via: "session" };
const health = { status: "ok", version: "x", sources: { a: "ok", b: "ok" } };
const j = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
const children = createRawSnippet(() => ({ render: () => "<p>page</p>" }));

let routes: Record<string, () => Response>;
const assign = vi.fn();
const settle = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
async function mount(props: Partial<{ me: Me; path: string; query: string }> = {}) {
  const r = render(Shell, { me, path: "/", query: "", assign, children, ...props });
  await settle();
  await settle();
  return r;
}
const nav = () => screen.getByRole("navigation", { name: "Main" });
const dot = () => document.querySelector(".sw-dot") as HTMLElement;

beforeEach(() => {
  routes = {
    "GET /health": () => j(200, health),
    "GET /v1/scopes": () => j(200, { scopes: [{ source: "superpipeline", id: "brd_01", name: "Board one", runs: 2 }, { source: "agentpod", id: "st_1", runs: 1 }] }),
    "POST /auth/logout": () => new Response(null, { status: 204 }),
  };
  vi.stubGlobal("fetch", vi.fn(async (u: string, init?: RequestInit) => routes[`${init?.method ?? "GET"} ${u}`]()));
  assign.mockClear();
  document.cookie = "vk_appearance=; Max-Age=0; Path=/";
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("Shell", () => {
  it("lists the views, boards and rubrics, without counts", async () => {
    await mount();
    const links = within(nav()).getAllByRole("link").map((a) => [a.textContent?.trim(), a.getAttribute("href")]);
    expect(links).toEqual([
      ["All", "/"],
      ["Needs verdict", "/?needs_verdict=true"],
      ["Failed", "/?status=failed"],
      ["Waiting", "/?status=waiting"],
      ["Board one", "/?scope=brd_01"],
      ["st_1", "/?scope=st_1"],
      ["Rubrics", "/rubrics"],
    ]);
    expect(within(nav()).getAllByRole("heading").map((h) => h.textContent)).toEqual(["Runs", "Boards", "Library"]);
  });

  it("has one home link", async () => {
    await mount();
    expect(screen.getAllByRole("link").filter((a) => a.getAttribute("href") === "/" && a.textContent?.includes("superwitness"))).toHaveLength(1);
  });

  it("leaves Boards out when the scopes fail", async () => {
    routes["GET /v1/scopes"] = () => j(500, { error: { code: "internal", message: "x" } });
    await mount();
    expect(within(nav()).queryByText("Boards")).toBeNull();
    within(nav()).getByText("Rubrics");
  });

  it.each([
    ["/", "", "All"],
    ["/", "needs_verdict=true", "Needs verdict"],
    ["/", "status=failed", "Failed"],
    ["/", "scope=brd_01", "Board one"],
    ["/rubrics", "", "Rubrics"],
    ["/rubrics/r1/2", "", "Rubrics"],
  ])("marks %s?%s current as %s", async (path, query, label) => {
    await mount({ path, query });
    const cur = within(nav()).getAllByRole("link").filter((a) => a.getAttribute("aria-current") === "page");
    expect(cur.map((a) => a.textContent?.trim())).toEqual([label]);
  });

  it("marks nothing current for two filters at once", async () => {
    await mount({ query: "status=failed&executor=x" });
    expect(nav().querySelector("[aria-current]")).toBeNull();
  });

  it("says Sources ok, with the detail in the title", async () => {
    await mount();
    expect(dot().textContent).toContain("Sources ok");
    expect(dot().getAttribute("title")).toBe("All sources answer");
    expect(dot().dataset.tone).toBe("good");
  });

  it("names the failing source when /health says one is down", async () => {
    routes["GET /health"] = () => j(200, { ...health, sources: { ...health.sources, logs: "timeout" } });
    await mount();
    expect(dot().textContent).toContain("Sources degraded");
    expect(dot().getAttribute("title")).toBe("logs: timeout");
  });

  it("says Sources: unknown when /health fails", async () => {
    routes["GET /health"] = () => { throw new TypeError("x"); };
    await mount();
    expect(dot().getAttribute("title")).toBe("Sources: unknown");
  });

  it("polls /health every 60 s", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await mount();
    const calls = () => (fetch as ReturnType<typeof vi.fn>).mock.calls.filter(([u]) => u === "/health").length;
    expect(calls()).toBe(1);
    routes["GET /health"] = () => j(200, { ...health, sources: { traces: "timeout" } });
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(calls()).toBe(2);
    expect(dot().getAttribute("title")).toBe("traces: timeout");
  });

  it("turns amber when an API call answers 503 store_unavailable, and recovers", async () => {
    routes["GET /v1/scopes"] = () => j(503, { error: { code: "store_unavailable", message: "down", retryable: true } });
    await mount();
    expect(dot().dataset.tone).toBe("warn");
    expect(dot().getAttribute("title")).toBe("The database is not answering");
    const { getJSON } = await import("../api");
    routes["GET /v1/x"] = () => j(200, {});
    await act(async () => { await getJSON("/v1/x"); });
    expect(dot().dataset.tone).toBe("good");
  });

  it("opens the phone menu and closes it on navigation", async () => {
    const r = await mount();
    const menu = screen.getByRole("button", { name: "Menu" });
    expect(menu).toHaveAttribute("aria-controls", "nav");
    expect(menu).toHaveAttribute("aria-expanded", "false");
    await fireEvent.click(menu);
    expect(menu).toHaveAttribute("aria-expanded", "true");
    expect(document.querySelector(".sw-shell")?.classList.contains("open")).toBe(true);
    await r.rerender({ me, path: "/rubrics", query: "", assign, children });
    expect(menu).toHaveAttribute("aria-expanded", "false");
  });

  it("shows who is signed in and their principal", async () => {
    await mount();
    // Once on the page (the chip); the open menu adds the principal under it.
    expect(screen.getAllByText("a@example.com")).toHaveLength(1);
    await fireEvent.click(screen.getByRole("button", { name: /a@example.com/ }));
    screen.getByText("prn_human01");
  });

  it("signs out on 204 by going to the start", async () => {
    await mount();
    await fireEvent.click(screen.getByRole("menuitem", { name: "Sign out" }));
    await settle();
    expect(assign).toHaveBeenCalledWith("/");
  });

  it("on 503 store_unavailable shows the Signed out page instead of the shell", async () => {
    routes["POST /auth/logout"] = () => j(503, { error: { code: "store_unavailable", message: "down", retryable: true } });
    await mount();
    await fireEvent.click(screen.getByRole("menuitem", { name: "Sign out" }));
    await settle();
    expect(assign).not.toHaveBeenCalled();
    screen.getByRole("heading", { name: "Signed out" });
    expect(screen.queryByRole("navigation", { name: "Main" })).toBeNull();
  });

  it.each([
    ["403 origin_mismatch", () => j(403, { error: { code: "origin_mismatch", message: "x" } }), "The request didn’t come from the app; reload the page first."],
    ["429 rate_limited", () => j(429, { error: { code: "rate_limited", message: "x" } }, { "Retry-After": "7" }), "Too many requests; wait 7 s."],
    ["a network failure", () => { throw new TypeError("x"); }, "superwitness could not be reached."],
    ["500 internal", () => j(500, { error: { code: "internal", message: "it broke" } }), "it broke"],
  ])("on %s stays signed in, says why, and retries", async (_n, answer, reason) => {
    routes["POST /auth/logout"] = answer as () => Response;
    await mount();
    await fireEvent.click(screen.getByRole("menuitem", { name: "Sign out" }));
    await settle();
    expect(assign).not.toHaveBeenCalled();
    nav();
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain("Sign out failed — try again.");
    expect(alert.textContent).toContain(reason);
    routes["POST /auth/logout"] = () => new Response(null, { status: 204 });
    await fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));
    await settle();
    expect(assign).toHaveBeenCalledWith("/");
  });

  it("offers no Sign out to a bearer caller", async () => {
    await mount({ me: { ...me, via: "bearer" } });
    expect(screen.queryByRole("menuitem", { name: "Sign out" })).toBeNull();
  });

  it("the Appearance control writes the cookie and paints <html>", async () => {
    appearance.init();
    await mount();
    await fireEvent.click(screen.getByRole("menuitem", { name: "Appearance" }));
    const panel = screen.getByRole("group", { name: "Appearance" });
    await fireEvent.click(within(panel).getByRole("button", { name: "Studio" }));
    await fireEvent.click(within(panel).getByRole("button", { name: "Dark" }));
    await fireEvent.click(within(panel).getByRole("button", { name: /^Violet/ }));
    expect(document.cookie).toMatch(/vk_appearance=studio\.dark\./);
    expect(document.cookie).toMatch(/superwitness:violet/);
    const html = document.documentElement;
    expect([html.dataset.vibe, html.dataset.theme, html.dataset.accent]).toEqual(["studio", "dark", "violet"]);
    expect(within(panel).getByRole("button", { name: "Studio" })).toHaveAttribute("aria-pressed", "true");
    appearance.dispose();
  });
});
