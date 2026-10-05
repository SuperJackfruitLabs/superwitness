import type { ApiError } from "../api";

// The verdict refusals people meet in the drawer, in plain words. already_superseded is the
// code internal/verdicts returns when the verdict being revised already has a successor.
export const REFUSALS: Record<string, string> = {
  unknown_rubric: "That rubric version no longer exists",
  supersedes_not_found: "The verdict you're revising no longer exists",
  supersedes_mismatch: "A revision must keep the same subject and rubric",
  idempotency_conflict: "This form was already submitted with different values; reopen it",
  already_superseded: "You've already revised this verdict",
};

export function refusalText(e: ApiError): string {
  return REFUSALS[e.code] ?? e.message;
}
