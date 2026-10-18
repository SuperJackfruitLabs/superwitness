<script lang="ts" module>
  export interface TabItem {
    value: string;
    label: string;
    href: string;
  }
</script>

<script lang="ts">
  import { appearance } from "../appearance.svelte";

  // Tabs are links (each tab is a URL) with the tab pattern: role tab, aria-selected, and
  // Left/Right/Home/End moving focus and activating. vibekit's Tabs has links (no tab role, no
  // arrows) or buttons (no hrefs), so this keeps vibekit's look (its vk-tabs classes) on our markup.
  let {
    label,
    items,
    value,
    onactivate,
  }: { label: string; items: TabItem[]; value: string; onactivate?: (href: string) => void } = $props();

  let list = $state<HTMLElement | null>(null);
  const links = () => [...(list?.querySelectorAll<HTMLAnchorElement>("a[role=tab]") ?? [])];

  $effect(() => {
    void value;
    const el = list?.querySelector<HTMLElement>('[aria-selected="true"]');
    el?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
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

<div class="vk-tabs vk-tabs--{appearance.value.vibe} sw-tabs" role="tablist" aria-label={label} bind:this={list}>
  {#each items as item, i (item.value)}
    <a
      class="vk-tabs__item vk-hit"
      href={item.href}
      role="tab"
      aria-selected={item.value === value ? "true" : "false"}
      tabindex={item.value === value ? 0 : -1}
      onkeydown={(e) => key(e, i)}>{item.label}</a
    >
  {/each}
</div>

<style>
  .sw-tabs { overflow-x: auto; scrollbar-width: thin; }
</style>
