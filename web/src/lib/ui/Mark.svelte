<script lang="ts">
  // The superwitness mark: an eye on the product tile (Daylight, Paper), on the accent (Studio),
  // a small accent square (Quiet). Decorative.
  let { size = 24 }: { size?: number } = $props();
</script>

<span class="sw-mark" aria-hidden="true"><svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" focusable="false"><path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" /></svg></span>

<style>
  .sw-mark { display: inline-grid; place-items: center; flex: none; width: 44px; height: 44px; border-radius: var(--vk-radius-md); background: var(--vk-product-superwitness-bg); color: var(--vk-product-superwitness-fg); }
  :global(:where([data-vibe="paper"])) .sw-mark { width: 28px; height: 28px; border-radius: 50%; }
  :global(:where([data-vibe="paper"])) .sw-mark svg { width: 16px; height: 16px; }
  :global(:where([data-vibe="studio"])) .sw-mark { width: 34px; height: 34px; border-radius: 0; background: var(--vk-color-accent); color: var(--vk-color-accent-fg); }
  :global(:where([data-vibe="studio"])) .sw-mark svg { width: 18px; height: 18px; }
  :global(:where([data-vibe="quiet"])) .sw-mark { width: 14px; height: 14px; border-radius: 3px; background: var(--vk-color-accent); }
  :global(:where([data-vibe="quiet"])) .sw-mark svg { display: none; }
</style>
