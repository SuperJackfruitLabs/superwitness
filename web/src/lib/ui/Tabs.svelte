<script lang="ts" module>
  export interface TabItem {
    value: string;
    label: string;
    href: string;
  }

  // tabEdges says which ends of a scrolling strip have tabs past them, for the fade that hints at them.
  export function tabEdges(scrollLeft: number, clientWidth: number, scrollWidth: number): string | undefined {
    const more = [scrollLeft > 1 && "left", scrollLeft + clientWidth < scrollWidth - 1 && "right"].filter(Boolean);
    return more.length ? more.join(" ") : undefined;
  }

  // tabScroll is where a strip scrolls to so the selected tab sits in view, centred when it can be.
  export function tabScroll(tabLeft: number, tabWidth: number, clientWidth: number, scrollWidth: number): number {
    const centred = tabLeft - (clientWidth - tabWidth) / 2;
    return Math.max(0, Math.min(centred, scrollWidth - clientWidth));
  }
</script>

<script lang="ts">
  // Tabs are links (each tab is a URL) with the tab pattern: role tab, aria-selected, and
  // Left/Right/Home/End moving focus and activating. vibekit's Tabs has links (no tab role, no
  // arrows) or buttons (no hrefs). On a narrow screen the strip scrolls, the selected tab is brought
  // into view (centred when it can be), and the edges with tabs past them fade (data-more).
  let {
    label,
    items,
    value,
    onactivate,
  }: { label: string; items: TabItem[]; value: string; onactivate?: (href: string) => void } = $props();

  let list = $state<HTMLElement | null>(null);
  let more = $state<string | undefined>();
  const links = () => [...(list?.querySelectorAll<HTMLAnchorElement>("a[role=tab]") ?? [])];

  $effect(() => {
    void value;
    const el = list;
    if (!el) return;
    const edges = () => (more = tabEdges(el.scrollLeft, el.clientWidth, el.scrollWidth));
    const sel = el.querySelector<HTMLElement>('[aria-selected="true"]');
    if (sel && el.scrollWidth > el.clientWidth) el.scrollLeft = tabScroll(sel.offsetLeft, sel.offsetWidth, el.clientWidth, el.scrollWidth);
    edges();
    el.addEventListener("scroll", edges, { passive: true });
    window.addEventListener("resize", edges);
    return () => {
      el.removeEventListener("scroll", edges);
      window.removeEventListener("resize", edges);
    };
  });

  function key(e: KeyboardEvent, i: number) {
    const n = items.length;
    const to = { ArrowRight: (i + 1) % n, ArrowLeft: (i - 1 + n) % n, Home: 0, End: n - 1 }[e.key];
    if (to === undefined) return;
    e.preventDefault();
    const a = links()[to];
    a?.focus();
    if (onactivate) onactivate(items[to].href);
    else a?.click();
  }
</script>

<div class="sw-tabs" role="tablist" aria-label={label} bind:this={list} data-more={more}>
  {#each items as item, i (item.value)}
    <a
      class="sw-tab"
      href={item.href}
      role="tab"
      aria-selected={item.value === value ? "true" : "false"}
      tabindex={item.value === value ? 0 : -1}
      onkeydown={(e) => key(e, i)}>{item.label}</a
    >
  {/each}
</div>

<style>
  /* Daylight (base): a raised pill strip; the selected tab ringed in the accent (SW-RunTrace-Daylight). */
  .sw-tabs {
    display: flex; gap: 6px; max-width: 100%; width: max-content; box-sizing: border-box; margin: 22px 0 0; padding: 6px;
    overflow-x: auto; scrollbar-width: none; border-radius: var(--vk-radius-pill); background: var(--vk-color-surface);
    box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground);
  }
  .sw-tabs::-webkit-scrollbar { display: none; }
  .sw-tabs[data-more="left"] { mask-image: linear-gradient(to right, transparent, #000 32px); }
  .sw-tabs[data-more="right"] { mask-image: linear-gradient(to left, transparent, #000 32px); }
  .sw-tabs[data-more="left right"] { mask-image: linear-gradient(to right, transparent, #000 32px, #000 calc(100% - 32px), transparent); }
  .sw-tab {
    display: inline-flex; align-items: center; flex: none; min-height: 44px; padding: 0 20px; border-radius: var(--vk-radius-pill);
    color: var(--vk-color-muted); font-weight: 600; text-decoration: none; white-space: nowrap;
  }
  .sw-tab:hover { color: var(--vk-color-text); background: var(--vk-color-raised); }
  .sw-tab:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: -3px; }
  .sw-tab[aria-selected="true"] { background: var(--vk-color-raised); color: var(--vk-color-text); font-weight: 800; box-shadow: inset 0 0 0 2px var(--vk-color-accent); }

  /* Paper: small-caps words on a double rule; the selected tab an italic serif, underlined. */
  :global(:where([data-vibe="paper"])) .sw-tabs { width: auto; gap: 30px; padding: 0; border-radius: 0; background: none; box-shadow: none; border-bottom: 3px double var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-tab {
    min-height: 48px; padding: 0 2px; border-radius: 0; margin-bottom: -3px; border-bottom: 3px solid transparent;
    font: 600 17px/1 var(--vk-font-body); font-variant-caps: all-small-caps; letter-spacing: 0.09em;
  }
  :global(:where([data-vibe="paper"])) .sw-tab:hover { background: none; }
  :global(:where([data-vibe="paper"])) .sw-tab[aria-selected="true"] { background: none; box-shadow: none; border-bottom-color: var(--vk-color-text); font: italic 500 21px/1 var(--vk-font-display); font-variant-caps: normal; letter-spacing: 0; }

  /* Studio: bold sans on a rule; the selected tab carries a 4px accent bar. */
  :global(:where([data-vibe="studio"])) .sw-tabs { width: auto; gap: 28px; margin-top: 0; padding: 0; border-radius: 0; background: none; box-shadow: none; border-bottom: 1px solid var(--vk-color-rule); }
  :global(:where([data-vibe="studio"])) .sw-tab { position: relative; min-height: 48px; padding: 0; border-radius: 0; font: 700 14px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-tab:hover { background: none; }
  :global(:where([data-vibe="studio"])) .sw-tab[aria-selected="true"] { background: none; box-shadow: inset 0 -4px 0 var(--vk-color-accent); font-weight: 800; }

  /* Quiet: underlined links on a hairline; the selected one plain, bold, with an accent bar. */
  :global(:where([data-vibe="quiet"])) .sw-tabs { width: auto; gap: 0 var(--vk-space-5, 24px); margin-top: var(--vk-space-4, 16px); padding: 0; border-radius: 0; background: none; box-shadow: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-tab { min-height: 48px; padding: 0; border-radius: 0; color: var(--vk-color-text); font-weight: 400; text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-tab:hover { background: none; }
  :global(:where([data-vibe="quiet"])) .sw-tab[aria-selected="true"] { background: none; font-weight: 700; text-decoration: none; box-shadow: inset 0 -4px 0 var(--vk-color-accent); }
</style>
