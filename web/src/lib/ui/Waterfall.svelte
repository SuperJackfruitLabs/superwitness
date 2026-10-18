<script lang="ts">
  import { duration } from "../format";
  import type { Row } from "../run/facts";
  import { spanStatus } from "../run/transcript";

  // Waterfall (SW-RunTrace-*): one row per span, sorted by start: a cite box, the name (a button
  // that opens the span) indented under its parent with its service, a bar placed by percent of the
  // whole trace (never thinner than a few pixels), and the duration in ms. The rows keep the old
  // DOM contract the e2e suite reads: ul.waterfall > li, label.cite, button.span-name.
  let {
    rows,
    total,
    ticked,
    canAdd,
    selected,
    ontick,
    onopen,
  }: {
    rows: Row[];
    total: number;
    ticked: (id: string) => boolean;
    canAdd: boolean;
    selected: string | null;
    ontick: (id: string) => boolean;
    onopen: (id: string) => void;
  } = $props();

  const ticks = $derived([0, 1, 2, 3, 4].map((i) => (i === 0 ? "0" : duration((total * i) / 4))));
</script>

<div class="sw-wf">
  <div class="sw-wf__axis" aria-hidden="true">
    <span></span><span></span>
    <span class="sw-wf__ticks">{#each ticks as t, i (i)}<span>{t}</span>{/each}</span>
    <span></span>
  </div>
  <ul class="waterfall">
    {#each rows as { span, depth, left, width } (span.span_id)}
      {@const on = ticked(span.span_id)}
      {@const error = spanStatus(span) === "ERROR"}
      <li class:on class:sel={span.span_id === selected}>
        <label class="cite">
          <input
            type="checkbox"
            aria-label="Cite {span.name} as evidence"
            checked={on}
            disabled={!on && !canAdd}
            onchange={(e) => {
              ontick(span.span_id);
              e.currentTarget.checked = ticked(span.span_id);
            }}
          />
        </label>
        <span class="sw-wf__name" style:--depth={depth}>
          <button
            type="button"
            class="span-name"
            title="{span.service} · {span.span_id}"
            aria-pressed={span.span_id === selected ? "true" : "false"}
            onclick={() => onopen(span.span_id)}>{span.name}</button
          >
          {#if error}<span class="sw-wf__err">ERROR</span>{/if}
          <span class="sw-wf__svc">{span.service}</span>
        </span>
        <span class="track"><span class="bar" class:err={error} style:left="{left}%" style:width="{width}%"></span></span>
        <span class="dur">{Math.round(span.duration_ms)} ms</span>
      </li>
    {/each}
  </ul>
</div>

<style>
  /* Daylight (base): zebra rows in rounded bands, accent bars on a soft track (SW-RunTrace-Daylight). */
  .sw-wf { min-width: 0; container-type: inline-size; }
  .sw-wf__axis, li { display: grid; grid-template-columns: 44px minmax(220px, 30%) minmax(0, 1fr) 96px; align-items: center; column-gap: 12px; }
  .sw-wf__axis { padding: 0 8px 8px; border-bottom: 2px solid var(--vk-color-line); color: var(--vk-color-muted); font: 600 var(--vk-text-sm) / 1.2 var(--vk-font-body); }
  .sw-wf__ticks { display: flex; justify-content: space-between; gap: 6px; font-variant-numeric: tabular-nums; white-space: nowrap; }
  ul { margin: 0; padding: 0; list-style: none; }
  li { min-height: 48px; padding: 0 8px; border-radius: var(--vk-radius-sm); }
  li:nth-child(odd) { background: var(--vk-color-bg); }
  li.on { background: var(--vk-color-accent-wash); }
  li.sel { box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  .cite { display: grid; place-items: center; width: 44px; height: 44px; cursor: pointer; }
  .cite input { width: 20px; height: 20px; margin: 0; accent-color: var(--vk-color-accent); }
  .sw-wf__name { display: flex; align-items: baseline; flex-wrap: wrap; gap: 2px 8px; min-width: 0; padding-left: calc(var(--depth, 0) * 18px); }
  .span-name {
    min-height: 44px; min-width: 0; padding: 0; border: 0; background: none; color: var(--vk-color-text); cursor: pointer; text-align: left;
    font: 700 1rem/1.3 var(--vk-font-body); text-decoration: underline; text-underline-offset: 3px; overflow-wrap: anywhere;
  }
  .span-name:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  .sw-wf__svc { color: var(--vk-color-muted); font-size: 0.8125rem; white-space: nowrap; }
  .sw-wf__err { padding: 1px 8px; border-radius: var(--vk-radius-pill); background: var(--sw-danger-wash); color: var(--sw-danger-text); font-size: 0.75rem; font-weight: 800; }
  .track { position: relative; height: 16px; border-radius: var(--vk-radius-pill); background: var(--vk-color-raised); }
  .bar { position: absolute; top: 0; bottom: 0; min-width: 6px; border-radius: var(--vk-radius-pill); background: var(--vk-color-accent); }
  .bar.err { background: var(--sw-danger); }
  .dur { text-align: right; font: 500 0.8125rem/1 var(--vk-font-mono); font-variant-numeric: tabular-nums; white-space: nowrap; }

  /* Paper: hairline rows, serif names with a tree lead, ink bars on a thin rule. */
  :global(:where([data-vibe="paper"])) .sw-wf__axis, :global(:where([data-vibe="paper"])) li { grid-template-columns: 44px minmax(0, 380px) minmax(0, 1fr) 112px; column-gap: 16px; }
  :global(:where([data-vibe="paper"])) .sw-wf__axis { padding: 0 0 6px; border-bottom: 1px solid var(--vk-color-text); font: 400 13px/1.2 var(--vk-font-mono); }
  :global(:where([data-vibe="paper"])) li { min-height: 52px; padding: 0; border-radius: 0; border-bottom: 1px solid var(--vk-color-line); background: none; }
  :global(:where([data-vibe="paper"])) li.on { background: color-mix(in srgb, var(--vk-color-text) 4%, transparent); }
  :global(:where([data-vibe="paper"])) li.sel { box-shadow: inset 3px 0 0 var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .cite input { width: 18px; height: 18px; accent-color: var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-wf__name { padding-left: calc(var(--depth, 0) * 22px); align-items: center; }
  :global(:where([data-vibe="paper"])) .span-name { font: 500 18px/1.2 var(--vk-font-display); text-decoration: none; }
  :global(:where([data-vibe="paper"])) .span-name:hover { text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="paper"])) li.sel .span-name { font-style: italic; text-decoration: underline; }
  :global(:where([data-vibe="paper"])) .sw-wf__err { padding: 0; background: none; font: 600 14px/1 var(--vk-font-body); font-variant-caps: all-small-caps; letter-spacing: 0.08em; }
  :global(:where([data-vibe="paper"])) .sw-wf__svc { display: none; }
  :global(:where([data-vibe="paper"])) .track { height: 16px; border-radius: 0; background: linear-gradient(var(--vk-color-line), var(--vk-color-line)) center / 100% 1px no-repeat; }
  :global(:where([data-vibe="paper"])) .bar { top: 3px; bottom: auto; height: 10px; min-width: 4px; border-radius: 1px; background: var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .bar.err { background: var(--sw-danger); }
  :global(:where([data-vibe="paper"])) .dur { font: 400 14px/1 var(--vk-font-mono); color: var(--vk-color-muted); }
  :global(:where([data-vibe="paper"])) li.on .dur { color: var(--vk-color-text); }

  /* Studio: a ruled grid, mono names, quarter gridlines in the lane, stage bars; an error is ink. */
  :global(:where([data-vibe="studio"])) .sw-wf__axis, :global(:where([data-vibe="studio"])) li { grid-template-columns: 44px minmax(0, 300px) minmax(0, 1fr) 96px; column-gap: 0; }
  :global(:where([data-vibe="studio"])) .sw-wf { border-top: 1.5px solid var(--vk-color-rule); }
  :global(:where([data-vibe="studio"])) .sw-wf__axis { min-height: 30px; padding: 0; border-bottom: 1px solid var(--vk-color-rule); font: 500 11px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) li { min-height: 48px; padding: 0; border-radius: 0; border-bottom: 1px solid var(--vk-color-line); background: none; }
  :global(:where([data-vibe="studio"])) li.on { background: var(--vk-color-accent-wash); }
  :global(:where([data-vibe="studio"])) li.sel { box-shadow: inset 0 0 0 2px var(--vk-color-accent); }
  :global(:where([data-vibe="studio"])) .sw-wf__name { padding-left: calc(var(--depth, 0) * 18px); align-items: center; }
  :global(:where([data-vibe="studio"])) .span-name { font: 600 14px var(--vk-font-mono); text-decoration: none; }
  :global(:where([data-vibe="studio"])) .span-name:hover { text-decoration: underline; }
  :global(:where([data-vibe="studio"])) .sw-wf__svc { font: 500 11px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-wf__err { padding: 2px 5px; border-radius: 0; background: var(--vk-color-text); color: var(--vk-color-bg); font: 700 10.5px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .track { align-self: stretch; height: auto; border-radius: 0; background: repeating-linear-gradient(90deg, var(--vk-color-line) 0 1px, transparent 1px 25%); }
  :global(:where([data-vibe="studio"])) .bar { top: 50%; bottom: auto; height: 14px; transform: translateY(-50%); min-width: 3px; border-radius: 0; background: var(--vk-stage-6, var(--vk-color-accent)); }
  :global(:where([data-vibe="studio"])) .bar.err { background: var(--vk-color-text); min-width: 4px; }
  :global(:where([data-vibe="studio"])) .dur { font: 500 13px var(--vk-font-mono); }

  /* Quiet: table-like rows with a ruled head, names over their service, ink bars; an error is striped. */
  :global(:where([data-vibe="quiet"])) .sw-wf__axis, :global(:where([data-vibe="quiet"])) li { grid-template-columns: 56px minmax(0, 38%) minmax(0, 1fr) 120px; column-gap: 12px; }
  :global(:where([data-vibe="quiet"])) .sw-wf__axis { padding: 8px 6px; border-bottom: 2px solid var(--vk-color-text); color: var(--vk-color-text); font: 400 1rem/1.4 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) li { min-height: 56px; padding: 6px; border-radius: 0; border-bottom: 1px solid var(--vk-color-line); background: none; }
  :global(:where([data-vibe="quiet"])) li.on { background: var(--vk-color-accent-wash); }
  :global(:where([data-vibe="quiet"])) li.sel { box-shadow: inset 4px 0 0 var(--vk-color-accent); }
  :global(:where([data-vibe="quiet"])) .cite input { width: 22px; height: 22px; }
  :global(:where([data-vibe="quiet"])) .sw-wf__name { flex-direction: column; align-items: flex-start; gap: 0; padding-left: calc(var(--depth, 0) * 24px); }
  :global(:where([data-vibe="quiet"])) .span-name { min-height: 32px; font: 700 1.125rem/1.4 var(--vk-font-body); text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-wf__svc { order: 2; font-size: 0.9375rem; }
  :global(:where([data-vibe="quiet"])) .sw-wf__err { order: 1; padding: 0; border-radius: 0; background: none; color: var(--vk-color-text); font-size: 1rem; font-weight: 700; }
  :global(:where([data-vibe="quiet"])) .track { border-radius: 0; }
  :global(:where([data-vibe="quiet"])) .bar { min-width: 4px; border-radius: 0; background: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .bar.err { min-width: 8px; background: repeating-linear-gradient(45deg, var(--vk-color-text) 0 3px, transparent 3px 6px); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .dur { font: 400 1.125rem/1.4 var(--vk-font-body); }

  /* Beside an open span pane the lanes are short: the axis keeps its ends, the service goes. */
  @container (max-width: 760px) {
    .sw-wf__ticks span:not(:first-child):not(:last-child) { display: none; }
    .sw-wf__svc { display: none; }
  }

  /* Phone: tick, name and ms on one line; the bar under them, across the row. */
  @media (max-width: 759px) {
    .sw-wf__axis { display: none; }
    li, :global(:where([data-vibe])) li {
      grid-template-columns: 44px minmax(0, 1fr) auto; grid-template-areas: "cite name dur" "cite track track";
      column-gap: 8px; row-gap: 2px; padding: 4px 6px 8px;
    }
    .cite { grid-area: cite; }
    .sw-wf__name { grid-area: name; }
    .track { grid-area: track; height: 10px; }
    :global(:where([data-vibe="studio"])) .track { height: 14px; }
    .dur { grid-area: dur; align-self: center; }
    .sw-wf__svc { display: none; }
  }
</style>
