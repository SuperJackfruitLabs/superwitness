import { onUnauthenticated } from "./api";

// session.expired turns true when any call answers 401: the layout then shows the signed-out page,
// whose Sign in link (signInHref) brings the person back to exactly where they were.
export const session = $state({ expired: false });

onUnauthenticated(() => {
  session.expired = true;
});

export const signInHref = (path: string, query: string) =>
  `/auth/login?next=${encodeURIComponent(path + (query ? `?${query}` : ""))}`;

// Automatic sign-in: a visit with no session goes straight to /auth/login, since the person is
// usually still signed in at the organization plane and comes back without a click. Two marks in
// localStorage stop it: one after a sign-out (the person chose to leave), and the time of the last
// attempt, so a sign-in that comes back without a session shows the Sign in page, never a loop.
const SIGNED_OUT = "sw_signed_out";
const ATTEMPT = "sw_auto_signin";
export const AUTO_SIGNIN_EVERY_MS = 60_000;

type Store = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const local = (): Store | null => {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
};

export function markSignedOut(store = local()) {
  try {
    store?.setItem(SIGNED_OUT, "1");
  } catch {}
}

// clearSignedOut runs when the person clicks Sign in: from then on, automatic sign-in is back.
export function clearSignedOut(store = local()) {
  try {
    store?.removeItem(SIGNED_OUT);
  } catch {}
}

// autoSignIn answers whether to go to sign-in now, and records the attempt when it does.
export function autoSignIn(store = local(), now = Date.now()): boolean {
  if (!store) return false;
  try {
    if (store.getItem(SIGNED_OUT)) return false;
    const last = Number(store.getItem(ATTEMPT) ?? 0);
    if (now - last < AUTO_SIGNIN_EVERY_MS) return false;
    store.setItem(ATTEMPT, String(now));
    return true;
  } catch {
    return false;
  }
}
