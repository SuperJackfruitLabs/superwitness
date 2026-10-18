import { getContext, setContext } from "svelte";
import type { Me } from "./types";

// Who is signed in, for pages inside the shell (the run page's Revise). The layout sets it once
// /v1/me has answered; pages only render after that.
const KEY = Symbol("me");

export function setMe(me: () => Me | null): void {
  setContext(KEY, me);
}

export function getMe(): () => Me | null {
  return getContext<(() => Me | null) | undefined>(KEY) ?? (() => null);
}
