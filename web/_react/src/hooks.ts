import { useCallback, useEffect, useState } from "react";
import { ApiError, getJSON } from "./api";

export interface Fetched<T> {
  data: T | null;
  error: ApiError | null;
  loading: boolean;
  reload: () => void;
}

// useJSON fetches path, again whenever it changes or reload is called. null fetches nothing.
export function useJSON<T>(path: string | null): Fetched<T> {
  const [state, setState] = useState<{ data: T | null; error: ApiError | null; loading: boolean }>({
    data: null,
    error: null,
    loading: path !== null,
  });
  const [n, setN] = useState(0);
  useEffect(() => {
    if (path === null) return;
    let live = true;
    setState((s) => ({ ...s, loading: true, error: null }));
    getJSON<T>(path).then(
      (data) => live && setState({ data, error: null, loading: false }),
      (error: ApiError) => live && setState({ data: null, error, loading: false }),
    );
    return () => {
      live = false;
    };
  }, [path, n]);
  const reload = useCallback(() => setN((x) => x + 1), []);
  return { ...state, reload };
}
