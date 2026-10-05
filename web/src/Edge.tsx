import { ApiError } from "./api";
import { Link } from "./router";

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

export function NotAuthorised() {
  return (
    <main className="edge">
      <h1>Not authorised</h1>
      <p>This account is not allowed to use this superwitness. Ask its operator to add you.</p>
    </main>
  );
}

export function DatabaseDown() {
  return (
    <section>
      <h1>Can’t reach the database</h1>
      <p className="muted">superwitness is running but its database is not answering. This page will work again when it does.</p>
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
export function ErrorView({ error, next = "/" }: { error: ApiError; next?: string }) {
  if (error.status === 401) return <SignedOut next={next} />;
  if (error.code === "not_authorised") return <NotAuthorised />;
  if (error.code === "store_unavailable") return <DatabaseDown />;
  if (error.code === "run_not_found") return <RunNotFound />;
  return (
    <section>
      <h1>Something went wrong</h1>
      <p className="refusal">{error.message}</p>
    </section>
  );
}
