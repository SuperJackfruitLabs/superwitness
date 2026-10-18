<script lang="ts">
  import type { Snippet } from "svelte";
  // Inspector is a non-modal side pane (the span pane): an aside with a head (title and Close), a
  // body that scrolls on its own and a foot that stays put. No scrim, no dialog role, Escape is not
  // captured: the page beside it keeps working. Daylight and Quiet make room for it (the run view
  // narrows, see RunView); Paper and Studio float it over the page's right edge with an edge and a
  // shadow. On a phone it fills the screen with its head and foot fixed. Below 1100px it floats in
  // every vibe.
  let { label, head, children, foot, onclose }: { label: string; head: Snippet; children: Snippet; foot?: Snippet; onclose: () => void } = $props();
</script>

<aside class="sw-inspector" aria-label={label}>
  <div class="sw-inspector__head">
    <div class="sw-inspector__title">{@render head()}</div>
    <button type="button" class="sw-inspector__close" onclick={onclose}
      ><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true" focusable="false"><path d="M6 6l12 12M18 6L6 18" /></svg
      >Close</button
    >
  </div>
  <div class="sw-inspector__body">{@render children()}</div>
  {#if foot}<div class="sw-inspector__foot">{@render foot()}</div>{/if}
</aside>

<style>
  /* Daylight (base): a raised card at the right, the page narrowed beside it. */
  .sw-inspector {
    position: fixed; z-index: 20; top: 16px; right: 16px; bottom: 16px; width: 480px; max-width: 100vw; box-sizing: border-box;
    display: flex; flex-direction: column; overflow: hidden; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface); color: var(--vk-color-text);
    box-shadow: inset 0 0 0 2px var(--vk-color-line), 0 24px 60px -20px var(--vk-color-ground), 0 var(--vk-lift, 3px) 0 var(--vk-color-ground);
  }
  .sw-inspector__head { display: flex; align-items: flex-start; gap: 12px; padding: 18px 16px 14px 22px; border-bottom: 2px solid var(--vk-color-line); }
  .sw-inspector__title { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .sw-inspector__close {
    flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 44px; padding: 0 14px; border: 0; border-radius: var(--vk-radius-pill);
    background: var(--vk-color-raised); color: var(--vk-color-text); font: 700 0.9375rem/1 var(--vk-font-body); cursor: pointer;
  }
  .sw-inspector__close:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  .sw-inspector__body { flex: 1; min-height: 0; overflow: auto; padding: 16px 22px 18px; display: flex; flex-direction: column; gap: 18px; }
  .sw-inspector__foot { display: flex; align-items: center; gap: 12px; padding: 12px 22px; border-top: 2px solid var(--vk-color-line); background: var(--vk-color-raised); }

  /* Paper: a sheet over the page's right edge, a double rule under its head. */
  :global(:where([data-vibe="paper"])) .sw-inspector { top: 0; right: 0; bottom: 0; width: 520px; border-radius: 0; background: var(--vk-color-bg); box-shadow: -1px 0 0 var(--vk-color-text), -24px 0 48px -24px rgb(0 0 0 / 0.35); }
  :global(:where([data-vibe="paper"])) .sw-inspector__head { padding: 18px 24px 12px 28px; border-bottom: 3px double var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-inspector__close { border-radius: var(--vk-radius-sm); background: none; box-shadow: inset 0 0 0 1px var(--vk-color-text); font-weight: 600; text-decoration: underline; text-underline-offset: 3px; }
  :global(:where([data-vibe="paper"])) .sw-inspector__body { padding: 16px 28px 18px; }
  :global(:where([data-vibe="paper"])) .sw-inspector__foot { padding: 12px 28px; border-top: 1px solid var(--vk-color-text); background: var(--vk-color-bg); }

  /* Studio: an ink-edged sheet over the page. */
  :global(:where([data-vibe="studio"])) .sw-inspector { top: 0; right: 0; bottom: 0; width: 520px; border-radius: 0; background: var(--vk-color-bg); box-shadow: -4px 0 0 var(--vk-color-text), -24px 0 48px -24px rgb(0 0 0 / 0.4); }
  :global(:where([data-vibe="studio"])) .sw-inspector__head { padding: 18px 18px 14px 26px; border-bottom: 1.5px solid var(--vk-color-rule); }
  :global(:where([data-vibe="studio"])) .sw-inspector__close { border-radius: 0; background: none; box-shadow: inset 0 0 0 1.5px var(--vk-color-text); font: 800 14px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-inspector__body { padding: 16px 26px 18px; }
  :global(:where([data-vibe="studio"])) .sw-inspector__foot { padding: 12px 26px; border-top: 1.5px solid var(--vk-color-rule); background: var(--vk-color-surface); }

  /* Quiet: a plain column at the right with a heavy rule, the page narrowed beside it. */
  :global(:where([data-vibe="quiet"])) .sw-inspector { top: 0; right: 0; bottom: 0; width: 480px; border-radius: 0; background: var(--vk-color-surface); box-shadow: -3px 0 0 var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-inspector__head { padding: 20px 20px 12px 26px; border-bottom: 0; }
  :global(:where([data-vibe="quiet"])) .sw-inspector__close { border-radius: var(--vk-radius-sm); background: none; box-shadow: inset 0 0 0 2px var(--vk-color-accent); font: 400 1rem var(--vk-font-body); text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-inspector__body { padding: 8px 26px 18px; }
  :global(:where([data-vibe="quiet"])) .sw-inspector__foot { margin: 0 26px; padding: 12px 0; border-top: 1px solid var(--vk-color-line); background: none; }

  /* Phone: the whole screen; the body scrolls between a fixed head and foot. */
  @media (max-width: 759px) {
    .sw-inspector, :global(:where([data-vibe])) .sw-inspector { inset: 0; width: auto; max-width: none; border-radius: 0; box-shadow: none; }
    .sw-inspector__head, :global(:where([data-vibe])) .sw-inspector__head { padding: 14px 16px 12px; }
    .sw-inspector__body, :global(:where([data-vibe])) .sw-inspector__body { padding: 14px 16px 18px; }
    .sw-inspector__foot, :global(:where([data-vibe])) .sw-inspector__foot { margin: 0; padding: 12px 16px; }
  }
</style>
