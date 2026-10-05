import { useEffect } from "react";
import { ApiError } from "./api";
import { Link } from "./router";
import { SignedOutLingering, SignOutFailed, useSignOut } from "./signout";

// How often a "Can't reach the database" page asks again on its own.
export const DATABASE_RETRY_MS = 30_000;

export function SignedOut({ next }: { next: string }) {
  return (
    <main className="edge">
      <h1>superwitness</h1>
      <p>Sign in to browse runs and record verdicts.</p>
      <p>
        <a className="button" href={`/auth/login?next=${encodeURIComponent(next)}`}>
          Sign in with AgentPod
        </a>
      </p>
    </main>
  );
}

// NotAuthorised is what a signed-in person sees once they are off the allowlist: their session
// still exists, so the page offers the way out.
export function NotAuthorised({ assign }: { assign?: (to: string) => void }) {
  const [out, signOut] = useSignOut(assign);
  if (out.kind === "lingering") return <SignedOutLingering />;
  return (
    <main className="edge">
      <h1>Not authorised</h1>
      <p>This account is not allowed to use this superwitness. Ask its operator to add you.</p>
      <p>
        <button className="button" onClick={signOut} disabled={out.kind === "busy"}>
          Sign out
        </button>
      </p>
      {out.kind === "failed" && <SignOutFailed reason={out.reason} retry={signOut} />}
    </main>
  );
}

// DatabaseDown asks again every DATABASE_RETRY_MS, and on demand, when given a retry.
export function DatabaseDown({ retry }: { retry?: () => void }) {
  useEffect(() => {
    if (!retry) return;
    const t = setInterval(retry, DATABASE_RETRY_MS);
    return () => clearInterval(t);
  }, [retry]);
  return (
    <section>
      <h1>Can’t reach the database</h1>
      <p className="muted">superwitness is running but its database is not answering. This page will work again when it does.</p>
      {retry && (
        <p>
          <button className="button" onClick={retry}>
            Try again
          </button>
        </p>
      )}
    </section>
  );
}

export function RunNotFound() {
  return (
    <section>
      <h1>Run not found</h1>
      <p className="muted">Neither superpipeline nor AgentPod knows this run.</p>
      <p>
        <Link to="/">All runs</Link>
      </p>
    </section>
  );
}

export function NotFound() {
  return (
    <section>
      <h1>Page not found</h1>
      <p>
        <Link to="/">All runs</Link>
      </p>
    </section>
  );
}

// ErrorView turns an API refusal into the page it calls for.
// retry, when given, reloads what failed; assign is for tests.
export function ErrorView({
  error,
  next = "/",
  retry,
  assign,
}: {
  error: ApiError;
  next?: string;
  retry?: () => void;
  assign?: (to: string) => void;
}) {
  if (error.status === 401) return <SignedOut next={next} />;
  if (error.code === "not_authorised") return <NotAuthorised assign={assign} />;
  if (error.code === "store_unavailable") return <DatabaseDown retry={retry} />;
  if (error.code === "run_not_found") return <RunNotFound />;
  if (error.code === "rate_limited") {
    const wait = error.retryAfter !== null ? ` in ${error.retryAfter} s` : " shortly";
    return (
      <section>
        <h1>Too many requests</h1>
        <p className="refusal">{`Too many requests — try again${wait}.`}</p>
      </section>
    );
  }
  if (error.code === "principal_unresolved") {
    return (
      <section>
        <h1>Can’t confirm who you are</h1>
        <p className="refusal">AgentPod can’t be reached to confirm who you are. Try again shortly.</p>
      </section>
    );
  }
  if (error.code === "origin_mismatch") {
    return (
      <section>
        <h1>Request refused</h1>
        <p className="refusal">This request didn’t come from the app. Reload the page and try again.</p>
      </section>
    );
  }
  return (
    <section>
      <h1>Something went wrong</h1>
      <p className="refusal">{error.message}</p>
    </section>
  );
}
