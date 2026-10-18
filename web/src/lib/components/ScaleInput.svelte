<script lang="ts">
  import type { Scale } from "../types";
  import { type Input, range, stepOf } from "../verdict/scale";

  // ScaleInput is the drawer's input for one recognised scale (SW-DrawerVariants-*): decision
  // toggle buttons, a label pick list, a score slider of twenty steps that is "not set" (thumb mid
  // track, faint) until touched, or a text area.
  let { scale, input, onchange, disabled = false }: { scale: Scale; input: Input; onchange: (i: Input) => void; disabled?: boolean } = $props();
  const bounds = $derived(range(scale));
</script>

{#if input.kind === "decision"}
  {@const choice = input.choice}
  <fieldset class="sw-scale sw-scale--decision" aria-label="Decision" {disabled}>
    <legend aria-hidden="true">Decision</legend>
    <div class="sw-seg">
      {#each scale.options ?? [] as o (o)}
        <button type="button" aria-pressed={choice === o} onclick={() => onchange({ kind: "decision", choice: o })}>{#if choice === o}<svg class="sw-seg__tick" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M3 8.5l3.2 3L13 4.5" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" /></svg>{/if}{o}</button>
      {/each}
    </div>
  </fieldset>
{:else if input.kind === "label"}
  <label class="sw-scale sw-field">
    <span class="sw-field__label">Label</span>
    <select value={input.choice ?? ""} {disabled} onchange={(e) => onchange({ kind: "label", choice: e.currentTarget.value || null })}>
      <option value="">Choose…</option>
      {#each scale.labels ?? [] as l (l)}
        <option value={l}>{l}</option>
      {/each}
    </select>
  </label>
{:else if input.kind === "score"}
  {@const cur = input}
  <label class="sw-scale sw-score" data-touched={cur.touched}>
    <span class="sw-score__head"><span class="sw-field__label">Score</span><output class:sw-score__unset={!cur.touched}>{cur.touched ? cur.x.toFixed(2) : "not set"}</output></span>
    <input
      type="range"
      min={bounds.lo}
      max={bounds.hi}
      step={stepOf(scale)}
      value={cur.x}
      {disabled}
      style:--sw-fill={cur.touched ? `${((cur.x - bounds.lo) / (bounds.hi - bounds.lo)) * 100}%` : "0%"}
      aria-valuetext={cur.touched ? undefined : "not set"}
      oninput={(e) => onchange({ kind: "score", x: Number(e.currentTarget.value), touched: true })}
      onclick={() => !cur.touched && onchange({ ...cur, touched: true })}
    />
    <span class="sw-score__ends" aria-hidden="true"><span>{bounds.lo}</span><span>{bounds.hi}</span></span>
  </label>
{:else if input.kind === "text"}
  <label class="sw-scale sw-field">
    <span class="sw-field__label">Verdict</span>
    <textarea value={input.text} {disabled} oninput={(e) => onchange({ kind: "text", text: e.currentTarget.value })}></textarea>
  </label>
{/if}

<style>
  fieldset { min-width: 0; margin: 0; padding: 0; border: 0; }
  legend { padding: 0; margin-bottom: 8px; font-weight: 700; }
  .sw-seg { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 8px; }
  .sw-seg button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 48px; padding: 0 16px; border: 0; border-radius: var(--vk-radius-pill); background: var(--vk-color-raised); color: var(--vk-color-text); font: 700 1.0625rem/1.2 var(--vk-font-body); box-shadow: inset 0 0 0 2px var(--vk-color-line); cursor: pointer; overflow-wrap: anywhere; }
  .sw-seg button[aria-pressed="true"] { background: var(--vk-color-accent); color: var(--vk-color-accent-fg); box-shadow: none; }
  .sw-seg button:focus-visible { outline: none; box-shadow: var(--vk-focus); }
  fieldset:disabled .sw-seg button { cursor: default; opacity: 0.75; }

  .sw-score { display: block; padding: 14px; border-radius: var(--vk-radius-md); box-shadow: inset 0 0 0 2px var(--vk-color-line); }
  .sw-score__head { display: flex; justify-content: space-between; align-items: baseline; }
  .sw-score output { font: 700 1.25rem/1 var(--vk-font-mono); }
  .sw-score output.sw-score__unset { font: italic 600 0.875rem/1 var(--vk-font-body); color: var(--vk-color-muted); }
  .sw-score input { display: block; width: 100%; height: 44px; margin: 0; background: none; appearance: none; -webkit-appearance: none; cursor: pointer; }
  .sw-score input::-webkit-slider-runnable-track { height: 8px; border-radius: var(--vk-radius-pill); background: linear-gradient(var(--vk-color-accent), var(--vk-color-accent)) 0 0 / var(--sw-fill) 100% no-repeat, var(--vk-color-raised); }
  .sw-score input::-moz-range-track { height: 8px; border-radius: var(--vk-radius-pill); background: var(--vk-color-raised); }
  .sw-score input::-moz-range-progress { height: 8px; border-radius: var(--vk-radius-pill); background: var(--vk-color-accent); }
  .sw-score input::-webkit-slider-thumb { -webkit-appearance: none; width: 24px; height: 24px; margin-top: -8px; border-radius: 50%; background: var(--vk-color-surface); box-shadow: inset 0 0 0 3px var(--vk-color-accent); }
  .sw-score input::-moz-range-thumb { width: 24px; height: 24px; border: 0; border-radius: 50%; background: var(--vk-color-surface); box-shadow: inset 0 0 0 3px var(--vk-color-accent); }
  .sw-score[data-touched="false"] input::-webkit-slider-thumb { box-shadow: inset 0 0 0 3px var(--vk-color-faint, var(--vk-color-muted)); }
  .sw-score[data-touched="false"] input::-moz-range-thumb { box-shadow: inset 0 0 0 3px var(--vk-color-faint, var(--vk-color-muted)); }
  .sw-score[data-touched="false"] input::-moz-range-progress { background: none; }
  .sw-score input:focus-visible { outline: none; }
  .sw-score input:focus-visible::-webkit-slider-thumb { box-shadow: inset 0 0 0 3px var(--vk-color-accent), var(--vk-focus); }
  .sw-score__ends { display: flex; justify-content: space-between; font: 500 0.75rem/1 var(--vk-font-mono); color: var(--vk-color-muted); }

  /* Paper: ruled square toggles, ink pressed. */
  :global(:where([data-vibe="paper"])) .sw-seg { grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 0; }
  :global(:where([data-vibe="paper"])) .sw-seg button { border-radius: 0; background: none; box-shadow: inset 0 0 0 1px var(--vk-color-text); font: italic 400 1.125rem/1.2 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-seg button[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="paper"])) legend { font: 600 12px/1.3 var(--vk-font-body); letter-spacing: 0.1em; text-transform: uppercase; color: var(--vk-color-muted); }
  :global(:where([data-vibe="paper"])) .sw-score { border-radius: 0; box-shadow: inset 0 0 0 1px var(--vk-color-line); }
  /* Studio: two square cells, the pressed one ink. */
  :global(:where([data-vibe="studio"])) .sw-seg { gap: 0; }
  :global(:where([data-vibe="studio"])) .sw-seg button { border-radius: 0; background: var(--vk-color-surface); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-seg button[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="studio"])) legend { font-size: 13px; }
  :global(:where([data-vibe="studio"])) .sw-score { border-radius: 0; box-shadow: inset 0 0 0 1px var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-score input::-webkit-slider-thumb { border-radius: 0; }
  :global(:where([data-vibe="studio"])) .sw-score input::-moz-range-thumb { border-radius: 0; }
  /* Quiet: plain bordered boxes; the pressed one on the accent wash with a 3px accent edge. */
  :global(:where([data-vibe="quiet"])) .sw-seg { grid-template-columns: repeat(auto-fill, minmax(110px, 130px)); gap: 10px; }
  :global(:where([data-vibe="quiet"])) .sw-seg button { border-radius: 0; background: none; box-shadow: inset 0 0 0 2px var(--vk-color-text); font-size: 1rem; }
  :global(:where([data-vibe="quiet"])) .sw-seg button[aria-pressed="true"] { background: var(--vk-color-accent-wash); color: var(--vk-color-text); box-shadow: inset 0 0 0 3px var(--vk-color-accent); }
  :global(:where([data-vibe="quiet"])) .sw-score { padding: 0; box-shadow: none; }
  :global(:where([data-vibe="quiet"])) .sw-score[data-touched="false"] input::-webkit-slider-thumb { box-shadow: none; outline: 2px dashed var(--vk-color-text); outline-offset: -2px; }
</style>
