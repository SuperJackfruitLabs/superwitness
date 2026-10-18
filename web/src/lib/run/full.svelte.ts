import { type ApiError, getJSON } from "../api";
import { itemFirstSeq, itemURL, type TranscriptItem } from "./transcript";

// fullItems holds the whole items read from the item route for one attempt, by first seq (the old
// useFullItems). The range the items were shown in rides along, so the hub resolves the item the
// page lists. Show full calls load; resolve swaps a cut item for its whole one once read.
export function fullItems(api: () => string, attempt: () => string, range?: () => { from: number; to: number | null }) {
  let full = $state.raw<Record<number, TranscriptItem>>({});
  let error = $state<string | null>(null);
  return {
    get error() {
      return error;
    },
    load(it: TranscriptItem) {
      const from = itemFirstSeq(it);
      error = null;
      getJSON<{ item: TranscriptItem }>(itemURL(api(), attempt(), from, range?.())).then(
        (r) => (full = { ...full, [from]: r.item }),
        (e: ApiError) => (error = e.message),
      );
    },
    resolve(it: TranscriptItem): TranscriptItem {
      return full[itemFirstSeq(it)] ?? it;
    },
  };
}
