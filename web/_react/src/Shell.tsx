import { type ReactNode, useEffect, useState, useSyncExternalStore } from "react";
import { getJSON, isStoreDown, subscribeStore } from "./api";
import { useJSON } from "./hooks";
import { Link, useLocation } from "./router";
import { SignedOutLingering, SignOutFailed, useSignOut } from "./signout";
import { sourcesState } from "./sources";
import { applyTheme, nextTheme, readTheme } from "./theme";
import type { Health, Me, ScopeEntry } from "./types";

export const VIEWS = [
  { label: "All", to: "/" },
  { label: "Needs verdict", to: "/?needs_verdict=true" },
  { label: "Failed", to: "/?status=failed" },
  { label: "Waiting", to: "/?status=waiting" },
];

function useHealth(): Health | null {
  const [h, setH] = useState<Health | null>(null);
  useEffect(() => {
    let live = true;
    const load = () =>
      getJSON<Health>("/health").then(
        (x) => live && setH(x),
        () => live && setH(null),
      );
    load();
    const t = setInterval(load, 60_000);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, []);
  return h;
}

// Sign out: see useSignOut for what each answer means.
export function Shell({ me, children, assign }: { me: Me; children: ReactNode; assign?: (to: string) => void }) {
  const { path, query } = useLocation();
  const here = query ? `${path}?${query}` : path;
  const [open, setOpen] = useState(false);
  const [theme, setTheme] = useState(readTheme);
  const [out, signOut] = useSignOut(assign);
  const scopes = useJSON<{ scopes: ScopeEntry[] }>("/v1/scopes");
  const storeDown = useSyncExternalStore(subscribeStore, isStoreDown, () => false);
  const dot = sourcesState(useHealth(), storeDown);
  useEffect(() => setOpen(false), [here]);

  if (out.kind === "lingering") return <SignedOutLingering />;

  return (
    <div className="shell">
      <header className="topbar">
        <Link to="/" className="wordmark">
          superwitness
        </Link>
        <button aria-expanded={open} aria-controls="nav" onClick={() => setOpen(!open)}>
          Menu
        </button>
      </header>
      <nav id="nav" className={open ? "side open" : "side"} aria-label="Main">
        <Link to="/" className="wordmark">
          superwitness
        </Link>
        <p className="navhead">Runs</p>
        {VIEWS.map((v) => (
          <Link key={v.to} to={v.to} aria-current={here === v.to ? "page" : undefined}>
            {v.label}
          </Link>
        ))}
        {(scopes.data?.scopes.length ?? 0) > 0 && <p className="navhead">Boards</p>}
        {scopes.data?.scopes.map((s) => {
          const to = `/?scope=${encodeURIComponent(s.id)}`;
          return (
            <Link key={`${s.source}/${s.id}`} to={to} aria-current={here === to ? "page" : undefined}>
              {s.name ?? s.id}
            </Link>
          );
        })}
        <p className="navhead">Library</p>
        <Link to="/rubrics" aria-current={path.startsWith("/rubrics") ? "page" : undefined}>
          Rubrics
        </Link>
        <div className="navfoot">
          <span className={`dot ${dot.tone}`} title={dot.label}>
            {dot.tone === "good" ? "Sources ok" : "Sources degraded"}
          </span>
          <button
            className="link"
            onClick={() => {
              const t = nextTheme(theme);
              applyTheme(t);
              setTheme(t);
            }}
          >
            Theme: {theme}
          </button>
          <span className="who" title={me.principal}>
            {me.email ?? me.principal}
          </span>
          {me.via === "session" && (
            <button className="link" onClick={signOut} disabled={out.kind === "busy"}>
              Sign out
            </button>
          )}
          {out.kind === "failed" && <SignOutFailed reason={out.reason} retry={signOut} />}
        </div>
      </nav>
      <main className="content">{children}</main>
    </div>
  );
}
