<script lang="ts" module>
  import type { RegistryRun } from "../types";
  // runHref is the run's page, for the sources that have one.
  export function runHref(r: RegistryRun): string | null {
    if (r.source !== "superpipeline" || !/^[A-Za-z0-9_-]{1,128}\/[A-Za-z0-9_-]{1,128}$/.test(r.external_ref)) return null;
    return `/runs/superpipeline/${r.external_ref}`;
  }
</script>

<script lang="ts">
  import { display, timing, verdictSummary } from "../format";
  import StatusPill from "../ui/StatusPill.svelte";

  // One run (SW-Runs-*): pill and title on the left, the meta on the right, one line from 1100px;
  // stacked below. A source with no run page is not a link and says so.
  let { run, now }: { run: RegistryRun; now: Date } = $props();
  const href = $derived(runHref(run));
  const v = $derived(run.latest_verdict);
</script>

{#snippet body()}
  <span class="sw-card__l">
    <StatusPill status={run.status} />
    <span class="sw-card__t">
      {#if run.title}{run.title}{:else}<code>{run.external_ref}</code>{/if}{#if !href}<span class="sw-card__tag">no run page yet</span>{/if}
    </span>
  </span>
  <span class="sw-card__meta">
    <span class="sw-card__time" class:dur={!!(run.started_at && run.ended_at)}>{timing(run, now)}</span>
    {#if run.scope}<span class="sw-card__scope">{display(run.scope.name ?? run.scope.id)}</span>{/if}
    {#if run.executor}<span class="sw-card__exec">{display(run.executor.name ?? run.executor.id)}</span>{/if}
    <span class="sw-card__v" class:none={!v}>{v ? `verdict: ${verdictSummary(v.value)}` : "no verdict"}</span>
  </span>
{/snippet}

{#if href}
  <a class="sw-card" {href}>{@render body()}</a>
{:else}
  <div class="sw-card sw-card--static" title="This source has no run page yet">{@render body()}</div>
{/if}

<style>
  /* Daylight is the base. */
  .sw-card {
    box-sizing: border-box; position: relative; display: flex; flex-direction: column; gap: 8px; min-height: 44px; padding: 14px 16px;
    border-radius: var(--vk-radius-md); background: var(--vk-color-surface); color: var(--vk-color-text);
    box-shadow: 0 var(--vk-lift) 0 var(--vk-color-ground); text-decoration: none;
  }
  a.sw-card:hover .sw-card__t { text-decoration: underline; text-underline-offset: 3px; }
  a.sw-card:focus-visible { outline: none; box-shadow: var(--vk-focus), 0 var(--vk-lift) 0 var(--vk-color-ground); }
  .sw-card__l { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; min-width: 0; }
  .sw-card__t { min-width: 0; font: 700 1.0625rem/1.35 var(--vk-font-display); overflow-wrap: anywhere; }
  .sw-card__t code { font: 600 0.95em var(--vk-font-mono); }
  .sw-card__tag {
    display: inline-block; margin-left: 10px; padding: 2px 10px; border-radius: var(--vk-radius-pill);
    background: var(--vk-color-surface); color: var(--vk-color-muted); font: 600 var(--vk-text-sm) / 1.3 var(--vk-font-body); vertical-align: 1px;
  }
  .sw-card__meta { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 14px; font-size: var(--vk-text-sm); color: var(--vk-color-muted); }
  .sw-card__meta > span { overflow-wrap: anywhere; }
  .sw-card__v { font-weight: 700; color: var(--vk-color-text); }
  .sw-card__v.none { font-weight: 600; color: var(--vk-color-muted); }
  .sw-card--static { background: var(--vk-color-raised); box-shadow: inset 0 0 0 2px var(--vk-color-line); }
  .sw-card--static .sw-card__t { font-weight: 600; }
  @media (min-width: 1100px) {
    .sw-card { flex-direction: row; align-items: center; gap: 16px; min-height: 64px; padding: 14px 20px 14px 18px; }
    .sw-card__l { flex: 1; flex-direction: row; align-items: center; gap: 14px; }
    .sw-card__meta { flex: none; max-width: 50%; flex-wrap: nowrap; white-space: nowrap; }
    /* A value too long for its half of the line is cut with an ellipsis instead of widening the page
       (the stacked card below 1100px shows it whole). */
    .sw-card__meta { min-width: 0; }
    .sw-card__meta > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  }

  /* Paper: ruled ledger rows; the pill in its own column; meta in small caps, scope first. */
  :global(:where([data-vibe="paper"])) .sw-card {
    border-radius: 0; background: transparent; box-shadow: none; padding: 14px 0; gap: 6px; border-bottom: 1px solid var(--vk-color-line);
  }
  :global(:where([data-vibe="paper"])) a.sw-card:focus-visible { box-shadow: none; outline: 2px solid var(--vk-color-text); outline-offset: 3px; }
  :global(:where([data-vibe="paper"])) .sw-card__t { font: 500 19px/1.3 var(--vk-font-display); }
  @media (min-width: 1100px) { :global(:where([data-vibe="paper"])) .sw-card__t { font-size: 22px; line-height: 1.25; } }
  :global(:where([data-vibe="paper"])) a.sw-card:hover .sw-card__t { text-decoration-thickness: 1px; text-underline-offset: 4px; }
  :global(:where([data-vibe="paper"])) .sw-card--static .sw-card__t { color: var(--vk-color-muted); }
  :global(:where([data-vibe="paper"])) .sw-card__tag {
    padding: 1px 7px 3px; border: 1px dashed var(--vk-color-faint); border-radius: var(--vk-radius-sm); background: transparent;
    font: italic 400 14px/1.2 var(--vk-font-display); vertical-align: 3px;
  }
  :global(:where([data-vibe="paper"])) .sw-card__meta { gap: 2px 0; font-size: 15px; font-variant-caps: all-small-caps; letter-spacing: 0.05em; }
  :global(:where([data-vibe="paper"])) .sw-card__scope { order: 1; }
  :global(:where([data-vibe="paper"])) .sw-card__exec { order: 2; }
  :global(:where([data-vibe="paper"])) .sw-card__time { order: 3; }
  :global(:where([data-vibe="paper"])) .sw-card__time.dur { font-variant-caps: normal; letter-spacing: 0; font-variant-numeric: tabular-nums; }
  :global(:where([data-vibe="paper"])) .sw-card__v { order: 4; }
  :global(:where([data-vibe="paper"])) .sw-card__v.none { font-weight: 400; color: var(--vk-color-muted); }
  :global(:where([data-vibe="paper"])) .sw-card__v:not(.none) { font-weight: 600; color: var(--vk-color-text); }
  /* With the reorder, the dot goes before every item but the first shown (scope, else the time). */
  :global(:where([data-vibe="paper"])) .sw-card__meta > span::before { content: "·"; padding: 0 9px; color: var(--vk-color-faint); }
  :global(:where([data-vibe="paper"])) .sw-card__meta > .sw-card__scope::before { content: none; }
  :global(:where([data-vibe="paper"])) .sw-card__meta:not(:has(.sw-card__scope)) > .sw-card__exec::before { content: none; }
  :global(:where([data-vibe="paper"])) .sw-card__meta:not(:has(.sw-card__scope, .sw-card__exec)) > .sw-card__time::before { content: none; }
  @media (min-width: 1100px) {
    :global(:where([data-vibe="paper"])) .sw-card {
      display: grid; grid-template-columns: 136px minmax(0, 1fr) minmax(0, 46%); gap: 20px; align-items: baseline; padding: 16px 8px 16px 0;
    }
    :global(:where([data-vibe="paper"])) .sw-card__l { display: contents; }
    :global(:where([data-vibe="paper"])) .sw-card :global(.sw-pill) { justify-self: start; }
    :global(:where([data-vibe="paper"])) .sw-card__meta { max-width: none; flex-wrap: wrap; white-space: normal; justify-content: flex-end; }
  }

  /* Studio: hairline rows; mono meta. */
  :global(:where([data-vibe="studio"])) .sw-card {
    border-radius: 0; background: transparent; box-shadow: none; padding: 12px 0; gap: 6px; border-bottom: 1px solid var(--vk-color-line);
  }
  :global(:where([data-vibe="studio"])) a.sw-card:hover { background: var(--vk-color-surface); }
  :global(:where([data-vibe="studio"])) a.sw-card:focus-visible { box-shadow: none; outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  :global(:where([data-vibe="studio"])) .sw-card__t { font: 700 16px/1.3 var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-card--static .sw-card__t { font-weight: 600; }
  :global(:where([data-vibe="studio"])) .sw-card__tag {
    display: inline-flex; align-items: center; height: 22px; padding: 0 6px; border-radius: 0; background: transparent;
    box-shadow: inset 0 0 0 1px var(--vk-color-line); font: 500 11.5px var(--vk-font-mono);
  }
  :global(:where([data-vibe="studio"])) .sw-card__meta { gap: 4px 18px; font: 500 12.5px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-card__v { font-weight: 500; }
  :global(:where([data-vibe="studio"])) .sw-card__v.none { font-weight: 500; color: var(--vk-color-muted); }
  @media (min-width: 1100px) {
    :global(:where([data-vibe="studio"])) .sw-card {
      display: grid; grid-template-columns: 112px minmax(0, 1fr) fit-content(50%); column-gap: 20px; align-items: center; min-height: 56px; padding: 10px 12px 10px 0;
    }
    :global(:where([data-vibe="studio"])) .sw-card__l { display: contents; }
    :global(:where([data-vibe="studio"])) .sw-card__meta { max-width: none; } /* the grid column holds it to half */
    :global(:where([data-vibe="studio"])) .sw-card__t { font-size: 15px; }
  }

  /* Quiet: rows between lines; the title underlined like a link; meta one muted line. */
  :global(:where([data-vibe="quiet"])) .sw-card {
    border-radius: 0; background: transparent; box-shadow: none; padding: 12px 4px; gap: 4px; border-top: 1px solid var(--vk-color-line);
  }
  :global(:where([data-vibe="quiet"])) a.sw-card:focus-visible { box-shadow: none; outline: 3px solid var(--vk-color-accent); outline-offset: 2px; border-radius: var(--vk-radius-sm); }
  :global(:where([data-vibe="quiet"])) .sw-card__t { font: 700 1.125rem/1.6 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) a.sw-card .sw-card__t { text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-card__tag {
    margin-left: 8px; padding: 0 6px; border: 1px dashed var(--vk-color-faint); border-radius: var(--vk-radius-sm); background: transparent;
    font: 400 0.9375rem/1.5 var(--vk-font-body); text-decoration: none;
  }
  :global(:where([data-vibe="quiet"])) .sw-card__meta { gap: 0; font-size: 1rem; }
  :global(:where([data-vibe="quiet"])) .sw-card__meta > span + span::before { content: " · "; white-space: pre; }
  :global(:where([data-vibe="quiet"])) .sw-card__v, :global(:where([data-vibe="quiet"])) .sw-card__v.none { font-weight: 400; color: var(--vk-color-muted); }
  :global(:where([data-vibe="quiet"])) .sw-card--static .sw-card__t { font-weight: 700; }
  @media (min-width: 1100px) {
    :global(:where([data-vibe="quiet"])) .sw-card {
      display: grid; grid-template-columns: auto minmax(0, 1fr) fit-content(50%); gap: 4px 16px; align-items: baseline; min-height: 56px;
    }
    :global(:where([data-vibe="quiet"])) .sw-card__l { display: contents; }
    :global(:where([data-vibe="quiet"])) .sw-card__meta { max-width: none; } /* the grid column holds it to half */
  }
</style>
