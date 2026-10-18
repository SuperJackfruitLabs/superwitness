import "@testing-library/jest-dom/vitest";

// Newer Node defines its own localStorage global (undefined without --localstorage-file), which
// shadows jsdom's. Give the tests a working one.
if (typeof localStorage === "undefined" || localStorage === null || typeof localStorage.clear !== "function") {
  const store = new Map<string, string>();
  const ls: Storage = {
    get length() {
      return store.size;
    },
    clear: () => store.clear(),
    getItem: (k) => store.get(k) ?? null,
    key: (i) => [...store.keys()][i] ?? null,
    removeItem: (k) => void store.delete(k),
    setItem: (k, v) => void store.set(k, String(v)),
  };
  Object.defineProperty(globalThis, "localStorage", { value: ls, configurable: true });
}
