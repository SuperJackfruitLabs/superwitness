<script lang="ts">
  import { onMount } from "svelte";
  import { getJSON, isStoreDown, subscribeStore } from "../api";
  import { sourcesState } from "../sources";
  import type { Health } from "../types";

  // How often the dot asks /health again.
  const HEALTH_MS = 60_000;

  let health = $state<Health | null>(null);
  let storeDown = $state(isStoreDown());
  const dot = $derived(sourcesState(health, storeDown));

  onMount(() => {
    let live = true;
    const load = () =>
      getJSON<Health>("/health").then(
        (h) => live && (health = h),
        () => live && (health = null),
      );
    load();
    const t = setInterval(load, HEALTH_MS);
    const off = subscribeStore(() => (storeDown = isStoreDown()));
    return () => {
      live = false;
      clearInterval(t);
      off();
    };
  });
</script>

<p class="sw-dot" data-tone={dot.tone} title={dot.label}>
  <i aria-hidden="true"></i><span>{dot.tone === "good" ? "Sources ok" : "Sources degraded"}</span>
</p>

<style>
  .sw-dot { display: flex; align-items: center; gap: 8px; min-height: 44px; margin: 0; padding: 0 12px; border-radius: var(--vk-radius-md); background: var(--vk-color-raised); font-size: var(--vk-text-sm); font-weight: 600; }
  .sw-dot i { flex: none; width: 9px; height: 9px; border-radius: 50%; background: var(--vk-color-accent); }
  .sw-dot[data-tone="warn"] { background: var(--sw-warn-wash); color: var(--sw-warn-text); }
  .sw-dot[data-tone="warn"] i { background: var(--sw-warn); }

  :global(:where([data-vibe="paper"])) .sw-dot { padding: 0; background: none; font-size: 14px; color: var(--vk-color-success-text, var(--vk-color-accent-text)); }
  :global(:where([data-vibe="paper"])) .sw-dot i { width: 8px; height: 8px; background: var(--vk-color-success, var(--vk-color-accent)); box-shadow: 0 0 0 3px var(--vk-color-success-wash, var(--vk-color-accent-wash)); }
  :global(:where([data-vibe="paper"])) .sw-dot[data-tone="warn"] { background: none; color: var(--sw-warn-text); }
  :global(:where([data-vibe="paper"])) .sw-dot[data-tone="warn"] i { box-shadow: 0 0 0 3px var(--sw-warn-wash); }

  :global(:where([data-vibe="studio"])) .sw-dot { min-height: 32px; padding: 0 6px; background: none; color: inherit; font: 500 12px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-dot i { width: 8px; height: 8px; border-radius: 0; background: var(--vk-stage-3, var(--vk-color-accent)); }
  :global(:where([data-vibe="studio"])) .sw-dot[data-tone="warn"] { background: none; color: var(--sw-warn-on-ink, var(--sw-warn)); }
  :global(:where([data-vibe="studio"])) .sw-dot[data-tone="warn"] i { background: var(--sw-warn-on-ink, var(--sw-warn)); }

  :global(:where([data-vibe="quiet"])) .sw-dot { padding: 0; background: none; font-size: 1rem; font-weight: 400; }
  :global(:where([data-vibe="quiet"])) .sw-dot i { width: 10px; height: 10px; }
  :global(:where([data-vibe="quiet"])) .sw-dot[data-tone="warn"] { background: none; color: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-dot[data-tone="warn"] i { border-radius: 0; background: none; box-shadow: inset 0 0 0 2px var(--vk-color-text); }
</style>
