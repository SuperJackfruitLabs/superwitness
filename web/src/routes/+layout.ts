// A client-only app: the Go binary serves index.html for every route (see svelte.config.js).
export const ssr = false;
export const prerender = false;
// /rubrics and /rubrics/ are the same page, as they were in the old router.
export const trailingSlash = "ignore";
