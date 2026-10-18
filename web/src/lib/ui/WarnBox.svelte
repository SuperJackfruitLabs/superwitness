<script lang="ts">
  import type { Snippet } from "svelte";
  // WarnBox is the amber "unavailable" box (SW-RunStates-*): a source did not answer, a read failed
  // and can be tried again. Daylight a wash with an inset bar, Paper a ruled wash, Studio an ink-bar
  // box with a drawn "!" mark (CSS, silent), Quiet a dashed border (its tones are ink). The words come from the caller;
  // a Try again button, when there is one, goes in actions.
  let { children, actions, role = "status" }: { children: Snippet; actions?: Snippet; role?: "status" | "alert" } = $props();
</script>

<div class="sw-warnbox" {role}>
  <div class="sw-warnbox__body">{@render children()}</div>
  {#if actions}<div class="sw-warnbox__act">{@render actions()}</div>{/if}
</div>

<style>
  .sw-warnbox {
    display: flex; align-items: center; flex-wrap: wrap; gap: 10px 16px; margin: 0; padding: 12px 16px; overflow-wrap: anywhere;
    border-radius: var(--vk-radius-md); background: var(--sw-warn-wash); color: var(--sw-warn-text); font-weight: 600;
    box-shadow: inset 4px 0 0 var(--sw-warn);
  }
  .sw-warnbox__body { flex: 1 1 24ch; min-width: 0; }
  .sw-warnbox__body :global(p) { margin: 0; }
  .sw-warnbox__act { flex: none; }
  :global(:where([data-vibe="paper"])) .sw-warnbox { flex-direction: column; align-items: flex-start; padding: 14px 18px; border-radius: 0; border: 1px solid var(--sw-warn); border-left-width: 4px; box-shadow: none; }
  :global(:where([data-vibe="paper"])) .sw-warnbox__body { flex: none; }
  :global(:where([data-vibe="studio"])) .sw-warnbox { align-items: flex-start; gap: 12px; padding: 12px 14px; border-radius: 0; box-shadow: none; border-left: 6px solid var(--sw-warn); color: var(--vk-color-text); font: 600 14.5px/1.45 var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-warnbox::before { content: "!" / ""; display: grid; place-items: center; flex: none; width: 20px; height: 20px; margin-top: 1px; background: var(--sw-warn); color: var(--vk-color-bg); font: 800 13px/1 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-warnbox { padding: 10px 14px; border: 2px dashed var(--vk-color-text); border-radius: var(--vk-radius-sm); background: none; box-shadow: none; color: var(--vk-color-text); font-weight: 400; }
</style>
