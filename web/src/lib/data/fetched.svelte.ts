import { ApiError, getJSON } from "../api";

export interface Fetched<T> {
  readonly data: T | null;
  readonly error: ApiError | null;
  readonly loading: boolean;
  reload(): void;
}

// fetched gets path() as JSON, again whenever path() changes or reload() is called; null fetches
// nothing. A generation counter drops any answer that is no longer the latest request's. Call it
// while a component initialises (or inside $effect.root): it creates an effect.
export function fetched<T>(path: () => string | null): Fetched<T> {
  let data = $state<T | null>(null);
  let error = $state<ApiError | null>(null);
  let loading = $state(path() !== null);
  let tries = $state(0);
  let gen = 0;
  $effect(() => {
    const p = path();
    void tries;
    const mine = ++gen;
    if (p === null) {
      loading = false;
      return;
    }
    loading = true;
    error = null;
    getJSON<T>(p).then(
      (d) => {
        if (gen !== mine) return;
        data = d;
        error = null;
        loading = false;
      },
      (e: ApiError) => {
        if (gen !== mine) return;
        data = null;
        error = e;
        loading = false;
      },
    );
    return () => {
      gen++;
    };
  });
  return {
    get data() {
      return data;
    },
    get error() {
      return error;
    },
    get loading() {
      return loading;
    },
    reload() {
      tries++;
    },
  };
}
