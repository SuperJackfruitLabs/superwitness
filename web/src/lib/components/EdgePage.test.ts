import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { ApiError } from "../api";
import EdgePage, { DATABASE_RETRY_MS } from "./EdgePage.svelte";
import NotFound from "./NotFound.svelte";

const view = (error: ApiError, props: Record<string, unknown> = {}) => render(EdgePage, { error, next: "/runs?x=1", ...props });
const h1 = () => screen.getByRole("heading", { level: 1 }).textContent;

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("EdgePage", () => {
  it("sends a 401 to sign in, remembering where the person was", () => {
    view(new ApiError(401, "unauthenticated", "x"));
    expect(h1()).toBe("superwitness");
    screen.getByText("Sign in to browse runs and record verdicts.");
    expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/auth/login?next=%2Fruns%3Fx%3D1");
    // A full page load: /auth is the Go server's, never a client-side route.
    expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute("data-sveltekit-reload");
  });
  it("says not authorised and offers sign out", () => {
    view(new ApiError(403, "not_authorised", "x"));
    expect(h1()).toBe("Not authorised");
    screen.getByText("This account is not allowed to use this superwitness. Ask its operator to add you.");
    screen.getByRole("button", { name: "Sign out" });
  });
  it("shows the lingering page when sign-out answers 503 store_unavailable", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { code: "store_unavailable", message: "x" } }), { status: 503 })));
    view(new ApiError(403, "not_authorised", "x"));
    await fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    await screen.findByRole("heading", { name: "Signed out" });
    screen.getByText(/You are signed out in this browser\. The database did not answer/);
    expect(screen.getByRole("link", { name: "Continue" })).toHaveAttribute("href", "/");
    expect(screen.getByRole("link", { name: "Continue" })).toHaveAttribute("data-sveltekit-reload");
  });
  it("says why a sign-out failed, with a retry", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { code: "rate_limited", message: "x" } }), { status: 429, headers: { "Retry-After": "5" } })));
    view(new ApiError(403, "not_authorised", "x"));
    await fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Sign out failed — try again. Too many requests; wait 5 s.");
    screen.getByRole("button", { name: "Try again" });
  });
  it("goes to the start after a sign-out", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    const assign = vi.fn();
    view(new ApiError(403, "not_authorised", "x"), { assign });
    await fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith("/"));
  });
  it("says the database is down and asks again every 30 s", async () => {
    vi.useFakeTimers();
    const retry = vi.fn();
    view(new ApiError(503, "store_unavailable", "x"), { retry });
    expect(h1()).toBe("Can’t reach the database");
    screen.getByText("superwitness is running but its database is not answering. This page will work again when it does.");
    expect(DATABASE_RETRY_MS).toBe(30_000);
    vi.advanceTimersByTime(29_999);
    expect(retry).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(retry).toHaveBeenCalledTimes(1);
    await fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalledTimes(2);
  });
  it("has no Try again without a retry", () => {
    view(new ApiError(503, "store_unavailable", "x"));
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });
  it("tells a rate-limited person when to retry", () => {
    view(new ApiError(429, "rate_limited", "x", true, 7));
    expect(h1()).toBe("Too many requests");
    expect(screen.getByRole("alert").textContent).toBe("Too many requests — try again in 7 s.");
    cleanup();
    view(new ApiError(429, "rate_limited", "x", true, null));
    expect(screen.getByRole("alert").textContent).toBe("Too many requests — try again shortly.");
  });
  it("explains an unresolved principal and a mismatched origin", () => {
    view(new ApiError(503, "principal_unresolved", "x", true));
    expect(h1()).toBe("Can’t confirm who you are");
    expect(screen.getByRole("alert").textContent).toBe("AgentPod can’t be reached to confirm who you are. Try again shortly.");
    cleanup();
    view(new ApiError(403, "origin_mismatch", "x"));
    expect(h1()).toBe("Request refused");
    expect(screen.getByRole("alert").textContent).toBe("This request didn’t come from the app. Reload the page and try again.");
  });
  it("falls back to the server's message, network included", () => {
    view(new ApiError(500, "internal", "HTTP 500"));
    expect(h1()).toBe("Something went wrong");
    expect(screen.getByRole("alert").textContent).toBe("HTTP 500");
    cleanup();
    view(new ApiError(0, "network", "superwitness could not be reached"));
    expect(screen.getByRole("alert").textContent).toBe("superwitness could not be reached");
  });
  it("says the run is not found, with a way back", () => {
    view(new ApiError(404, "run_not_found", "x"));
    expect(h1()).toBe("Run not found");
    screen.getByText("Neither superpipeline nor AgentPod knows this run.");
    expect(screen.getByRole("link", { name: "All runs" })).toHaveAttribute("href", "/");
  });
  it("says the rubric is not found, with a link to the rubrics", () => {
    view(new ApiError(404, "rubric_not_found", "no rubric a version 1"));
    expect(h1()).toBe("Rubric not found");
    expect(screen.getByRole("link", { name: "Rubrics" })).toHaveAttribute("href", "/rubrics");
  });
});

describe("NotFound", () => {
  it("says the page is not found and links to all runs", () => {
    render(NotFound);
    expect(h1()).toBe("Page not found");
    expect(screen.getByRole("link", { name: "All runs" })).toHaveAttribute("href", "/");
  });
});
