// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { navigate } from "./router";
import { Shell } from "./Shell";
import type { Me } from "./types";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const me: Me = { principal: "prn_human01", kind: "human", email: "a@example.com", via: "session" };
const health = { status: "ok", version: "x", sources: { a: "ok", b: "ok", c: "ok", d: "ok", e: "ok" } };
const j = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

let routes: Record<string, () => Response>;
let root: Root;
let el: HTMLElement;
const assign = vi.fn();

async function mount() {
  await act(async () => root.render(<Shell me={me} assign={assign}><p>page</p></Shell>));
  await act(async () => {});
}
const click = (e: Element | null | undefined) => act(async () => void (e as HTMLElement).dispatchEvent(new MouseEvent("click", { bubbles: true, button: 0 })));
const button = (text: string) => [...el.querySelectorAll("button")].find((b) => b.textContent?.startsWith(text));

beforeEach(() => {
  routes = {
    "GET /health": () => j(200, health),
    "GET /v1/scopes": () => j(200, { scopes: [{ source: "superpipeline", id: "brd_01", name: "Board one", runs: 2 }] }),
    "POST /auth/logout": () => new Response(null, { status: 204 }),
  };
  vi.stubGlobal("fetch", vi.fn(async (u: string, init?: RequestInit) => routes[`${init?.method ?? "GET"} ${u}`]()));
  const store = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
  });
  delete document.documentElement.dataset.theme;
  navigate("/", true);
  assign.mockClear();
  el = document.createElement("div");
  document.body.append(el);
  root = createRoot(el);
});
afterEach(async () => {
  await act(async () => root.unmount());
  el.remove();
  vi.unstubAllGlobals();
});

describe("Shell", () => {
  it("lists the views, boards and rubrics, with a green dot when all sources answer", async () => {
    await mount();
    const links = [...el.querySelectorAll("nav a")].map((a) => a.getAttribute("href"));
    expect(links).toEqual(expect.arrayContaining(["/", "/?needs_verdict=true", "/?status=failed", "/?status=waiting", "/?scope=brd_01", "/rubrics"]));
    expect(el.querySelector(".dot")?.className).toContain("good");
  });

  it("turns the dot amber, naming the source, when /health says one is down", async () => {
    routes["GET /health"] = () => j(200, { ...health, sources: { ...health.sources, logs: "timeout" } });
    await mount();
    expect(el.querySelector(".dot")?.className).toContain("warn");
    expect(el.querySelector(".dot")?.getAttribute("title")).toBe("logs: timeout");
  });

  it("turns the dot amber when an API call answers 503 store_unavailable", async () => {
    routes["GET /v1/scopes"] = () => j(503, { error: { code: "store_unavailable", message: "down", retryable: true } });
    await mount();
    expect(el.querySelector(".dot")?.className).toContain("warn");
  });

  it("cycles the theme system, light, dark, and keeps it in localStorage", async () => {
    await mount();
    expect(button("Theme")?.textContent).toBe("Theme: system");
    await click(button("Theme"));
    expect([button("Theme")?.textContent, document.documentElement.dataset.theme, localStorage.getItem("superwitness.theme")]).toEqual(["Theme: light", "light", "light"]);
    await click(button("Theme"));
    expect(document.documentElement.dataset.theme).toBe("dark");
    await click(button("Theme"));
    expect([document.documentElement.dataset.theme, localStorage.getItem("superwitness.theme")]).toEqual([undefined, null]);
  });

  it("opens the menu and closes it when navigating", async () => {
    await mount();
    const nav = el.querySelector("nav")!;
    expect(nav.className).toBe("side");
    await click(button("Menu"));
    expect(nav.className).toBe("side open");
    expect(button("Menu")?.getAttribute("aria-expanded")).toBe("true");
    await click(el.querySelector('nav a[href="/rubrics"]'));
    expect(nav.className).toBe("side");
  });

  it("signs out on 204 by going to the start", async () => {
    await mount();
    await click(button("Sign out"));
    expect(assign).toHaveBeenCalledWith("/");
  });

  it("on 503 store_unavailable treats the user as signed out and says the session may outlive it", async () => {
    routes["POST /auth/logout"] = () => j(503, { error: { code: "store_unavailable", message: "down", retryable: true } });
    await mount();
    await click(button("Sign out"));
    expect(assign).not.toHaveBeenCalled();
    expect(el.textContent).toContain("signed out in this browser");
    expect(el.textContent).toContain("may outlive");
    expect(el.querySelector("nav")).toBeNull();
  });

  it("offers no Sign out to a bearer caller", async () => {
    await act(async () => root.render(<Shell me={{ ...me, via: "bearer" }}><p>x</p></Shell>));
    expect(button("Sign out")).toBeUndefined();
  });
});
