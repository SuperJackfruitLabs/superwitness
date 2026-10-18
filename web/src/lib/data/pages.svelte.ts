import type { ApiError } from "../api";

export interface Page<T> {
  items: T[];
  next: string | null;
}

export interface Pages<T, P extends Page<T> = Page<T>> {
  readonly items: T[];
  readonly next: string | null;
  // The first page as the API sent it, for fields beyond items and next (the runs page's counts).
  readonly first: P | null;
  readonly error: ApiError | null;
  readonly moreError: ApiError | null;
  readonly loading: boolean;
  more(): void;
  retry(): void;
}

// pages loads a cursor-paged list. Every first load and every load-more carries a generation; when
// key() changes (another run, another filter), retry() is called or the owner is destroyed, the
// generation moves on and a response from an older one is dropped. A first-load failure is `error`;
// a load-more failure is `moreError` and leaves what was already loaded in place. Call it while a
// component initialises (or inside $effect.root): it creates an effect.
export function pages<T, P extends Page<T> = Page<T>>(key: () => string, fetchPage: (cursor: string | null) => Promise<P>): Pages<T, P> {
  let items = $state<T[]>([]);
  let next = $state<string | null>(null);
  let first = $state.raw<P | null>(null);
  let error = $state<ApiError | null>(null);
  let moreError = $state<ApiError | null>(null);
  let loading = $state(true);
  let tries = $state(0);
  let gen = 0;
  let busy = false;
  $effect(() => {
    void key();
    void tries;
    const mine = ++gen;
    busy = false;
    items = [];
    next = null;
    first = null;
    error = null;
    moreError = null;
    loading = true;
    fetchPage(null).then(
      (p) => {
        if (gen !== mine) return;
        items = p.items;
        next = p.next;
        first = p;
        loading = false;
      },
      (e: ApiError) => {
        if (gen !== mine) return;
        error = e;
        loading = false;
      },
    );
    return () => {
      gen++;
    };
  });
  return {
    get items() {
      return items;
    },
    get next() {
      return next;
    },
    get first() {
      return first;
    },
    get error() {
      return error;
    },
    get moreError() {
      return moreError;
    },
    get loading() {
      return loading;
    },
    more() {
      if (!next || busy) return;
      const mine = gen;
      busy = true;
      moreError = null;
      fetchPage(next).then(
        (p) => {
          if (gen !== mine) return;
          busy = false;
          items = [...items, ...p.items];
          next = p.next;
        },
        (e: ApiError) => {
          if (gen !== mine) return;
          busy = false;
          moreError = e;
        },
      );
    },
    retry() {
      tries++;
    },
  };
}
