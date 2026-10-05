import { useState } from "react";
import { ApiError, postJSON } from "./api";

export type SignOutState = { kind: "idle" } | { kind: "busy" } | { kind: "lingering" } | { kind: "failed"; reason: string };

const reload = (to: string) => window.location.assign(to);

// signOutReason says in plain words why a sign-out failed.
export function signOutReason(e: unknown): string {
  if (!(e instanceof ApiError)) return "Something went wrong.";
  if (e.code === "rate_limited")
    return e.retryAfter !== null ? `Too many requests; wait ${e.retryAfter} s.` : "Too many requests; wait a moment.";
  if (e.code === "network") return "superwitness could not be reached.";
  if (e.code === "origin_mismatch") return "The request didn’t come from the app; reload the page first.";
  return e.message || `The server answered HTTP ${e.status}.`;
}

// useSignOut ends the session. 204: go to the start. 503 store_unavailable: the server has
// already cleared the cookie, so this browser is signed out, but the session row may still be in
// the database, so the caller says so instead of pretending. Anything else (403, 429, network):
// the person is still signed in, so stay and say why, with a retry.
export function useSignOut(assign: (to: string) => void = reload): [SignOutState, () => Promise<void>] {
  const [state, setState] = useState<SignOutState>({ kind: "idle" });
  async function signOut() {
    setState({ kind: "busy" });
    try {
      await postJSON("/auth/logout");
    } catch (e) {
      if (e instanceof ApiError && e.status === 503 && e.code === "store_unavailable") setState({ kind: "lingering" });
      else setState({ kind: "failed", reason: signOutReason(e) });
      return;
    }
    assign("/");
  }
  return [state, signOut];
}

export function SignedOutLingering() {
  return (
    <main className="edge">
      <h1>Signed out</h1>
      <p>
        You are signed out in this browser. The database did not answer, so the server-side session may outlive this
        sign-out. On a shared computer, sign in again later and sign out once more.
      </p>
      <p>
        <a className="button" href="/">
          Continue
        </a>
      </p>
    </main>
  );
}

export function SignOutFailed({ reason, retry }: { reason: string; retry: () => void }) {
  return (
    <p className="refusal" role="alert">
      Sign out failed — try again. {reason}{" "}
      <button className="link" onClick={retry}>
        Try again
      </button>
    </p>
  );
}
