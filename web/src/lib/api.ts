// Every call the app makes. Session cookies ride along; the browser adds Origin to POSTs.

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly retryable: boolean = false,
    readonly retryAfter: number | null = null,
  ) {
    super(message);
  }
}

// storeDown is true after an API call answered 503 store_unavailable, until one succeeds. The
// shell's sources dot reads it (the database is a source /health does not make fatal).
let storeDown = false;
const storeListeners = new Set<() => void>();
function setStoreDown(v: boolean) {
  if (v === storeDown) return;
  storeDown = v;
  storeListeners.forEach((f) => f());
}
export function subscribeStore(f: () => void) {
  storeListeners.add(f);
  return () => {
    storeListeners.delete(f);
  };
}
export const isStoreDown = () => storeDown;

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let r: Response;
  try {
    r = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: body === undefined ? {} : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, "network", "superwitness could not be reached");
  }
  if (r.status === 204) {
    setStoreDown(false);
    return undefined as T;
  }
  const data = await r.json().catch(() => null);
  if (!r.ok) {
    const e = data?.error ?? {};
    if (r.status === 503 && e.code === "store_unavailable") setStoreDown(true);
    const ra = Number(r.headers.get("Retry-After"));
    throw new ApiError(
      r.status,
      e.code ?? `http_${r.status}`,
      e.message ?? `HTTP ${r.status}`,
      e.retryable === true,
      Number.isFinite(ra) && ra > 0 ? ra : null,
    );
  }
  // /health stays 200 through a database outage (the dot reads it for sources), so it proves nothing here.
  if (path !== "/health") setStoreDown(false);
  return data as T;
}

export const getJSON = <T>(path: string) => request<T>("GET", path);
export const postJSON = <T>(path: string, body?: unknown) => request<T>("POST", path, body);
