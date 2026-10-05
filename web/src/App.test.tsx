// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { DATABASE_RETRY_MS } from "./Edge";
import { navigate } from "./router";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const j = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const me = { principal: "prn_human01", kind: "human", email: "a@example.com", via: "session" };
const down = () => j(503, { error: { code: "store_unavailable", message: "down", retryable: true } });
const notAuthorised = () => j(403, { error: { code: "not_authorised", message: "not on the list" } });

let routes: Record<string, () => Response>;
let calls: string[];
let root: Root;
let el: HTMLElement;
const assign = vi.fn();

async function mount() {
  await act(async () => root.render(<App assign={assign} />));
  await act(async () => {});
}
const settle = () => act(async () => {});
const click = (e: Element | null | undefined) => act(async () => void (e as HTMLElement).dispatchEvent(new MouseEvent("click", { bubbles: true, button: 0 })));
const button = (text: string) => [...el.querySelectorAll("button")].find((b) => b.textContent?.startsWith(text));
const meCalls = () => calls.filter((c) => c === "GET /v1/me").length;

beforeEach(() => {
  calls = [];
  routes = {
    "GET /v1/me": down,
    "GET /health": () => j(200, { status: "ok", version: "x", sources: {} }),
    "GET /v1/scopes": () => j(200, { scopes: [] }),
    "GET /v1/runs": () => j(200, { runs: [], next_cursor: null }),
    "POST /auth/logout": () => new Response(null, { status: 204 }),
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (u: string, init?: RequestInit) => {
      const k = `${init?.method ?? "GET"} ${u}`;
      calls.push(k);
      return (routes[k] ?? (() => j(200, {})))();
    }),
  );
  vi.stubGlobal("localStorage", { getItem: () => null, setItem: () => {}, removeItem: () => {} });
  navigate("/", true);
  assign.mockClear();
  el = document.createElement("div");
  document.body.append(el);
  root = createRoot(el);
});
afterEach(async () => {
  await act(async () => root.unmount());
  el.remove();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("App when the database is down", () => {
  it("asks /v1/me again from Try again, and shows the app once it answers", async () => {
    await mount();
    expect(el.textContent).toContain("Can’t reach the database");
    routes["GET /v1/me"] = () => j(200, me);
    await click(button("Try again"));
    await settle();
    expect(meCalls()).toBe(2);
    expect(el.querySelector("nav")).not.toBeNull();
  });

  it(`asks /v1/me again on its own every ${DATABASE_RETRY_MS / 1000} s`, async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    await mount();
    expect(meCalls()).toBe(1);
    await act(async () => void vi.advanceTimersByTime(DATABASE_RETRY_MS - 1));
    expect(meCalls()).toBe(1);
    await act(async () => void vi.advanceTimersByTime(1));
    await settle();
    expect(meCalls()).toBe(2);
    expect(el.textContent).toContain("Can’t reach the database"); // still down: keeps asking
    routes["GET /v1/me"] = () => j(200, me);
    await act(async () => void vi.advanceTimersByTime(DATABASE_RETRY_MS));
    await settle();
    expect(meCalls()).toBe(3);
    expect(el.querySelector("nav")).not.toBeNull();
  });
});

describe("App when the person is not authorised", () => {
  it("offers Sign out, which goes to the start on 204", async () => {
    routes["GET /v1/me"] = notAuthorised;
    await mount();
    expect(el.textContent).toContain("Not authorised");
    await click(button("Sign out"));
    expect(calls).toContain("POST /auth/logout");
    expect(assign).toHaveBeenCalledWith("/");
  });

  it("stays and says why when sign-out fails, then retries", async () => {
    routes["GET /v1/me"] = notAuthorised;
    routes["POST /auth/logout"] = () =>
      new Response(JSON.stringify({ error: { code: "rate_limited", message: "slow down", retryable: true } }), { status: 429, headers: { "Retry-After": "4" } });
    await mount();
    await click(button("Sign out"));
    expect(assign).not.toHaveBeenCalled();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain("Sign out failed — try again. Too many requests; wait 4 s.");
    routes["POST /auth/logout"] = () => new Response(null, { status: 204 });
    await click(button("Try again"));
    expect(assign).toHaveBeenCalledWith("/");
  });

  it("on 503 store_unavailable says the session may outlive the sign-out", async () => {
    routes["GET /v1/me"] = notAuthorised;
    routes["POST /auth/logout"] = down;
    await mount();
    await click(button("Sign out"));
    expect(assign).not.toHaveBeenCalled();
    expect(el.textContent).toContain("may outlive");
  });
});
