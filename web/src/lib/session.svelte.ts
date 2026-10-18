import { onUnauthenticated } from "./api";

// session.expired turns true when any call answers 401: the layout then shows the signed-out page,
// whose Sign in link (signInHref) brings the person back to exactly where they were.
export const session = $state({ expired: false });

onUnauthenticated(() => {
  session.expired = true;
});

export const signInHref = (path: string, query: string) =>
  `/auth/login?next=${encodeURIComponent(path + (query ? `?${query}` : ""))}`;
