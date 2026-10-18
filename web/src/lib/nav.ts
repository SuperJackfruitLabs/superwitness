import { goto } from "$app/navigation";

// navigate moves within the app the way a link does: a URL change, no full load. The page keeps its
// scroll and focus (opening a span or switching a tab is not a new page).
export function navigate(to: string): void {
  void goto(to, { noScroll: true, keepFocus: true });
}
