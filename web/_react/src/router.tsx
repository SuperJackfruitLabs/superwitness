import { type AnchorHTMLAttributes, type MouseEvent, useSyncExternalStore } from "react";

// A small client-side router: the URL is the state, and Link changes it without a reload.

const listeners = new Set<() => void>();

function subscribe(f: () => void) {
  listeners.add(f);
  window.addEventListener("popstate", f);
  return () => {
    listeners.delete(f);
    window.removeEventListener("popstate", f);
  };
}

const snapshot = () => window.location.pathname + window.location.search;

// useLocation is the current path and query string (without "?"), as strings so they can be
// effect dependencies.
export function useLocation(): { path: string; query: string } {
  const href = useSyncExternalStore(subscribe, snapshot, () => "/");
  const i = href.indexOf("?");
  return i < 0 ? { path: href, query: "" } : { path: href.slice(0, i), query: href.slice(i + 1) };
}

export function navigate(to: string, replace = false) {
  if (replace) history.replaceState(null, "", to);
  else history.pushState(null, "", to);
  listeners.forEach((f) => f());
}

export function Link({ to, onClick, ...rest }: { to: string } & AnchorHTMLAttributes<HTMLAnchorElement>) {
  return (
    <a
      href={to}
      {...rest}
      onClick={(e: MouseEvent<HTMLAnchorElement>) => {
        onClick?.(e);
        if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
        e.preventDefault();
        navigate(to);
        window.scrollTo(0, 0);
      }}
    />
  );
}

export type Route =
  | { name: "runs" }
  | { name: "run"; board: string; run: string }
  | { name: "rubrics" }
  | { name: "rubric"; id: string; version: number }
  | { name: "notfound" };

export function match(path: string): Route {
  if (path === "/") return { name: "runs" };
  let m = /^\/runs\/superpipeline\/([A-Za-z0-9_-]{1,128})\/([A-Za-z0-9_-]{1,128})\/?$/.exec(path);
  if (m) return { name: "run", board: m[1], run: m[2] };
  if (/^\/rubrics\/?$/.test(path)) return { name: "rubrics" };
  m = /^\/rubrics\/([A-Za-z0-9_.-]{1,64})\/([1-9][0-9]{0,8})\/?$/.exec(path);
  if (m) return { name: "rubric", id: m[1], version: Number(m[2]) };
  return { name: "notfound" };
}
