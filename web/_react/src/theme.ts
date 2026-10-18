// The theme toggle: system (the default), light or dark. The choice is this browser's alone.

export type Theme = "system" | "light" | "dark";
const key = "superwitness.theme";

export function readTheme(): Theme {
  try {
    const v = localStorage.getItem(key);
    return v === "light" || v === "dark" ? v : "system";
  } catch {
    return "system";
  }
}

export function applyTheme(t: Theme) {
  const el = document.documentElement;
  if (t === "system") delete el.dataset.theme;
  else el.dataset.theme = t;
  try {
    if (t === "system") localStorage.removeItem(key);
    else localStorage.setItem(key, t);
  } catch {
    // private mode: the choice lasts until the tab closes
  }
}

export const nextTheme = (t: Theme): Theme => (t === "system" ? "light" : t === "light" ? "dark" : "system");
