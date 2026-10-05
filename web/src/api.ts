// Every call the app makes. Session cookies ride along; the browser adds Origin to POSTs.

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

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
  if (r.status === 204) return undefined as T;
  const data = await r.json().catch(() => null);
  if (!r.ok) {
    const e = data?.error ?? {};
    throw new ApiError(r.status, e.code ?? `http_${r.status}`, e.message ?? `HTTP ${r.status}`);
  }
  return data as T;
}

export const getJSON = <T>(path: string) => request<T>("GET", path);
export const postJSON = <T>(path: string, body?: unknown) => request<T>("POST", path, body);
