<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import { getJSON } from "../api";
  import { fetched } from "../data/fetched.svelte";
  import { pages } from "../data/pages.svelte";
  import type { Evidence } from "../evidence.svelte";
  import { list } from "../format";
  import { fullItems } from "../run/full.svelte";
  import { holds, itemRange, parseSeq, rangeRef, runURL, showValue, spanFor, type TranscriptItem, type TranscriptPage, transcriptURL } from "../run/transcript";
  import type { Doc, Span } from "../types";
  import Empty from "../ui/Empty.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import Loading from "../ui/Loading.svelte";
  import TranscriptRefusal from "./TranscriptRefusal.svelte";
  import Value from "./Value.svelte";

  // TranscriptAttempt is one attempt's transcript: cards in seq order, paged by cursor. A ?seq= card
  // is outlined and scrolled to the centre; the tab pages forward (at most MAX_CHASE pages) to reach
  // it. Each card cites its range as evidence.
  let { api, page, attempt, seq, evidence }: { api: string; page: string; attempt: Doc; seq: string | null; evidence: Evidence } = $props();

  const KIND_LABEL: Record<string, string> = {
    prompt: "Prompt", message: "Agent message", reasoning: "Reasoning", tool_call: "Tool call", permission: "Permission",
    state: "State", error: "Error", other: "Event",
  };
  // How many pages the tab will load to reach a seq named in the URL.
  const MAX_CHASE = 20;

  const id = $derived<string>(attempt.id);
  const tx = pages<TranscriptItem>(
    () => `${api}|${id}`,
    (cursor) => getJSON<TranscriptPage>(transcriptURL(api, id, { cursor })).then((p) => ({ items: list(p.items), next: p.next_cursor ?? null })),
  );
  const spans = fetched<{ spans: Span[] }>(() => `${api}/spans?limit=500`);
  const full = fullItems(() => api, () => id, () => ({ from: attempt.seq_from, to: attempt.seq_to }));
  const target = $derived(parseSeq(seq));
  const found = $derived(target ? tx.items.find((it) => holds(itemRange(it), target.from)) : undefined);

  // The chase budget is per target: following another ?seq= link starts a fresh 20 pages.
  let chased = 0;
  let chasedFor: string | null = null;
  $effect(() => {
    if (seq !== chasedFor) {
      chasedFor = seq;
      chased = 0;
    }
    if (target && !found && tx.next && !tx.loading && !tx.busy && !tx.moreError && chased < MAX_CHASE) {
      chased++;
      tx.more();
    }
  });
  let targetEl = $state<HTMLLIElement | null>(null);
  $effect(() => {
    targetEl?.scrollIntoView?.({ block: "center" });
  });

  const session = $derived<string>(attempt.session_id);
</script>

{#snippet body(it: TranscriptItem, raw: TranscriptItem)}
  {#if it.kind === "prompt"}
    <Value v={it.text} prose onfull={() => full.load(raw)} />
    {#if list(it.images).length > 0}<p class="sw-step__muted">Images: {list(it.images).map((i) => i.name).join(", ")}</p>{/if}
  {:else if it.kind === "message"}
    <Value v={it.text} prose onfull={() => full.load(raw)} />
  {:else if it.kind === "reasoning"}
    <details>
      <summary>Reasoning</summary>
      <Value v={it.text} prose onfull={() => full.load(raw)} />
    </details>
  {:else if it.kind === "tool_call"}
    <details open={it.status === "failed"}>
      <summary>{showValue(it.title ?? "tool call")} · {it.tool_kind ?? "other"} · {it.status ?? "unknown"}</summary>
      <h4>Input</h4>
      <Value v={it.input} onfull={() => full.load(raw)} />
      <h4>Output</h4>
      <Value v={it.output} onfull={() => full.load(raw)} />
    </details>
  {:else if it.kind === "permission"}
    {@const options = Array.isArray(it.options) ? (it.options as { name?: string; optionId?: string }[]) : []}
    {#if it.title !== undefined}<p class="sw-step__title">{showValue(it.title)}</p>{/if}
    <p>Asked to choose: {options.map((o) => o.name ?? o.optionId).join(" / ") || "no options"}</p>
    <p>Outcome: <span class="sw-step__mono">{it.outcome ?? "pending"}</span></p>
  {:else if it.kind === "state"}
    <p>{it.status ?? "unknown"}{it.reason ? ` · ${it.reason}` : ""}</p>
  {:else if it.kind === "error"}
    <ErrorPanel message={`${it.error_kind ?? "error"}: ${showValue(it.message ?? "")}`} />
  {:else}
    <p class="sw-step__muted">An event of type {it.type ?? it.kind}, shown by type only.</p>
  {/if}
{/snippet}

{#if tx.error}
  <TranscriptRefusal error={tx.error} onretry={tx.retry} />
{:else if tx.loading}
  <Loading label="Loading the transcript…" />
{:else}
  <div class="sw-tx">
    {#if attempt.seq_to === null}<Empty>Attempt still running; refresh for more</Empty>{/if}
    {#if tx.items.length === 0}
      <Empty>No session content in this attempt.</Empty>
    {:else}
      <ol class="transcript">
        {#each tx.items as raw (`${raw.kind}-${itemRange(raw).from}`)}
          {@const it = full.resolve(raw)}
          {@const r = itemRange(it)}
          {@const s = spanFor(list(spans.data?.spans), [attempt], id, r.from, r.to)}
          {@const ref = rangeRef(session, r.from, r.to)}
          {@const cited = evidence.has(ref)}
          {@const isTarget = found === raw}
          {@const failed = it.kind === "tool_call" && it.status === "failed"}
          {#if isTarget}
            <li bind:this={targetEl} class="sw-step" class:failed class:agent={it.kind === "message"} aria-current="true" data-kind={it.kind}>
              {@render card(it, raw, r, s, ref, cited, failed)}
            </li>
          {:else}
            <li class="sw-step" class:failed class:agent={it.kind === "message"} data-kind={it.kind}>
              {@render card(it, raw, r, s, ref, cited, failed)}
            </li>
          {/if}
        {/each}
      </ol>
    {/if}
    {#if full.error}<ErrorPanel message={full.error} />{/if}
    {#if tx.moreError}<ErrorPanel message={tx.moreError.message} />{/if}
    {#if tx.next}
      <p class="sw-tx__more"><Button variant="secondary" busy={tx.busy} onclick={tx.more}>{tx.moreError ? "Try loading more again" : "Load more"}</Button></p>
    {/if}
  </div>
{/if}

{#snippet card(it: TranscriptItem, raw: TranscriptItem, r: { from: number; to: number }, s: Span | null, ref: ReturnType<typeof rangeRef>, cited: boolean, failed: boolean)}
  <div class="card-head">
    <strong class="sw-step__kind">{KIND_LABEL[it.kind] ?? it.kind}</strong>
    <span class="sw-step__seq">{r.from === r.to ? `#${r.from}` : `#${r.from}–${r.to}`}</span>
    {#if it.partial}<span class="sw-step__note">started earlier</span>{/if}
    {#if (it.redactions ?? 0) > 0}<span class="sw-step__chip">{it.redactions} redacted</span>{/if}
    {#if failed}<span class="sw-step__fail">failed</span>{/if}
    <span class="sw-step__acts">
      {#if s}<a class="sw-step__span" href={runURL(page, { span: s.span_id })}>span ↗</a>{/if}
      <button type="button" class="sw-step__cite" aria-pressed={cited ? "true" : "false"} disabled={!cited && !evidence.canAdd} onclick={() => evidence.toggle(ref)}>{cited ? "Cited" : "Cite"}</button>
    </span>
  </div>
  <div class="sw-step__body">{@render body(it, raw)}</div>
{/snippet}

<style>
  .sw-tx { display: flex; flex-direction: column; gap: 14px; max-width: 920px; min-width: 0; }
  .transcript { display: flex; flex-direction: column; gap: 14px; margin: 0; padding: 0; list-style: none; min-width: 0; }
  .sw-step {
    position: relative; min-width: 0; padding: 16px 20px 16px 22px; border-radius: var(--vk-radius-md);
    background: var(--vk-color-surface); box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); overflow-wrap: anywhere;
    scroll-margin: 96px;
  }
  .sw-step.agent { background: var(--vk-color-accent-wash, var(--vk-color-raised)); }
  .sw-step.failed { box-shadow: inset 5px 0 0 var(--sw-danger), 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); }
  .sw-step[aria-current="true"] { outline: 3px solid var(--vk-color-accent); outline-offset: 0; }
  .card-head { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; min-width: 0; }
  .sw-step__kind { font: 800 1rem/1.2 var(--vk-font-display); }
  .sw-step__seq { font: 500 0.75rem/1 var(--vk-font-mono); color: var(--vk-color-muted); }
  .sw-step__note { font-size: 0.8125rem; color: var(--vk-color-muted); font-style: italic; }
  .sw-step__chip { padding: 2px 10px; border-radius: var(--vk-radius-pill); background: var(--vk-color-raised); font-size: 0.8125rem; font-weight: 700; color: var(--vk-color-muted); }
  .sw-step__fail {
    display: inline-flex; align-items: center; gap: 6px; padding: 2px 10px; border-radius: var(--vk-radius-pill);
    background: var(--sw-danger-wash); color: var(--sw-danger-text); box-shadow: inset 0 0 0 1px var(--sw-danger); font-size: 0.8125rem; font-weight: 700;
  }
  .sw-step__fail::before { content: "" / ""; width: 8px; height: 8px; border-radius: 50%; background: var(--sw-danger); }
  .sw-step__acts { display: flex; align-items: center; gap: 6px; margin-left: auto; }
  .sw-step__span { display: inline-flex; align-items: center; min-height: 44px; padding: 0 10px; font-size: var(--vk-text-sm); font-weight: 700; color: inherit; text-underline-offset: 3px; }
  .sw-step__cite {
    display: inline-flex; align-items: center; min-height: 44px; padding: 0 16px; border: 0; border-radius: var(--vk-radius-pill);
    background: var(--vk-color-raised); color: var(--vk-color-text); font: 700 var(--vk-text-sm)/1 var(--vk-font-body); cursor: pointer;
    box-shadow: inset 0 0 0 2px var(--vk-color-line);
  }
  .sw-step__cite[aria-pressed="true"] { background: var(--vk-color-accent); color: var(--vk-color-accent-fg); box-shadow: none; }
  .sw-step__cite:disabled { opacity: 0.5; cursor: not-allowed; }
  .sw-step__cite:focus-visible, .sw-step__span:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  .sw-step__body { margin-top: 6px; min-width: 0; }
  .sw-step__body p { margin: 4px 0; }
  .sw-step__title { font-weight: 700; }
  .sw-step__muted { color: var(--vk-color-muted); }
  .sw-step__mono { font: 500 0.875rem var(--vk-font-mono); }
  .sw-step__body h4 { margin: 10px 0 0; font: 700 0.8125rem/1.3 var(--vk-font-body); color: var(--vk-color-muted); }
  summary { display: flex; align-items: center; min-height: 44px; cursor: pointer; font-weight: 700; }
  summary:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  .sw-tx__more { display: flex; justify-content: center; margin: 4px 0 0; }

  /* Paper: a ledger. Kind, seq and notes in a left column in small caps; the body to the right. */
  :global(:where([data-vibe="paper"])) .transcript { gap: 0; border-top: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-step { display: grid; grid-template-columns: 180px minmax(0, 1fr); column-gap: 18px; padding: 18px 16px; border-radius: 0; background: none; box-shadow: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-step.agent { background: none; }
  :global(:where([data-vibe="paper"])) .sw-step.failed { box-shadow: inset 3px 0 0 var(--sw-danger); }
  :global(:where([data-vibe="paper"])) .card-head,
  :global(:where([data-vibe="studio"])) .card-head { display: contents; }
  :global(:where([data-vibe="paper"])) .card-head > :not(.sw-step__acts),
  :global(:where([data-vibe="studio"])) .card-head > :not(.sw-step__acts) { grid-column: 1; justify-self: start; }
  /* The actions sit at the body's top right; a float keeps the body's first lines clear of them. */
  :global(:where([data-vibe="paper"])) .sw-step__acts,
  :global(:where([data-vibe="studio"])) .sw-step__acts { position: absolute; top: 14px; right: 16px; margin: 0; }

  :global(:where([data-vibe="paper"])) .sw-step__body,
  :global(:where([data-vibe="studio"])) .sw-step__body { grid-column: 2; grid-row: 1 / span 6; margin-top: 0; }
  :global(:where([data-vibe="paper"])) .sw-step__body::before,
  :global(:where([data-vibe="studio"])) .sw-step__body::before { content: ""; float: right; width: 150px; height: 40px; }
  :global(:where([data-vibe="paper"])) .sw-step__title { font: 500 19px/1.3 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-step__kind { font: 600 13px/1.4 var(--vk-font-body); font-variant-caps: all-small-caps; letter-spacing: 0.1em; font-size: 15px; }
  :global(:where([data-vibe="paper"])) .sw-step__seq { font-size: 13px; color: var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-step__chip { padding: 0; background: none; font: italic 400 14px var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-step__fail { padding: 0; background: none; box-shadow: none; font: 600 12px var(--vk-font-body); letter-spacing: 0.08em; text-transform: uppercase; }
  :global(:where([data-vibe="paper"])) .sw-step__fail::before { border-radius: 0; }
  :global(:where([data-vibe="paper"])) .sw-step__cite { border-radius: var(--vk-radius-sm); background: var(--vk-color-surface); box-shadow: inset 0 0 0 1px var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-step__cite[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="paper"])) summary { font: 400 19px/1.3 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-step__body h4 { font: 600 12px var(--vk-font-body); letter-spacing: 0.1em; text-transform: uppercase; color: var(--vk-color-text); }

  /* Studio: numbered rows; the seq set large in the left column. */
  :global(:where([data-vibe="studio"])) .transcript { gap: 0; border-top: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-step { display: grid; grid-template-columns: 120px minmax(0, 1fr); column-gap: 20px; padding: 18px 0; border-radius: 0; background: none; box-shadow: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-step.agent { background: none; }
  :global(:where([data-vibe="studio"])) .sw-step.failed { box-shadow: inset 6px 0 0 var(--vk-color-text); padding-left: 14px; }
  :global(:where([data-vibe="studio"])) .sw-step[aria-current="true"] { outline-width: 3px; padding-inline: 14px; }
  :global(:where([data-vibe="studio"])) .sw-step__kind { font: 800 12px var(--vk-font-body); letter-spacing: 0.08em; text-transform: uppercase; color: var(--vk-color-muted); }
  :global(:where([data-vibe="studio"])) .sw-step__seq { font: 800 22px/1.1 var(--vk-font-body); color: var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-step__chip { padding: 0; background: none; border-radius: 0; font: 500 12px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-step__fail { padding: 1px 6px; border-radius: 0; background: var(--vk-color-text); color: var(--vk-color-bg); box-shadow: none; font: 700 12px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-step__fail::before { content: "×" / ""; width: auto; height: auto; background: none; }
  :global(:where([data-vibe="studio"])) .sw-step__acts { right: 0; }
  :global(:where([data-vibe="studio"])) .sw-step.failed .sw-step__acts, :global(:where([data-vibe="studio"])) .sw-step[aria-current="true"] .sw-step__acts { right: 14px; }
  :global(:where([data-vibe="studio"])) .sw-step__cite { border-radius: 0; background: var(--vk-color-surface); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-step__cite[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="studio"])) summary { font: 800 16px var(--vk-font-body); }

  /* Quiet: rules between steps, underlined links, square buttons. */
  :global(:where([data-vibe="quiet"])) .transcript { gap: 0; border-top: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-step { padding: 16px; border-radius: 0; background: none; box-shadow: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-step.agent { background: none; }
  :global(:where([data-vibe="quiet"])) .sw-step.failed { box-shadow: inset 4px 0 0 var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-step[aria-current="true"] { background: var(--vk-color-accent-wash, var(--vk-color-raised)); outline-color: var(--vk-color-accent); }
  :global(:where([data-vibe="quiet"])) .sw-step__kind { font: 700 1.125rem/1.3 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-step__seq { font-size: 1rem; }
  :global(:where([data-vibe="quiet"])) .sw-step__chip { padding: 0; background: none; font-weight: 400; font-size: 1rem; }
  :global(:where([data-vibe="quiet"])) .sw-step__fail { padding: 0 6px; border-radius: 0; background: none; color: var(--vk-color-text); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-step__fail::before { border-radius: 0; background: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-step__cite { border-radius: 0; background: var(--vk-color-surface); box-shadow: inset 0 0 0 2px var(--vk-color-text); font-weight: 400; font-size: 1rem; }
  :global(:where([data-vibe="quiet"])) .sw-step__cite[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); font-weight: 700; }
  :global(:where([data-vibe="quiet"])) summary { font-weight: 400; text-decoration: underline; text-underline-offset: 3px; }

  @media (max-width: 759px) {
    .sw-step { padding: 14px; }
    :global(:where([data-vibe="paper"])) .sw-step,
    :global(:where([data-vibe="studio"])) .sw-step { display: block; padding-inline: 0; }
    :global(:where([data-vibe="paper"])) .sw-step__acts,
    :global(:where([data-vibe="studio"])) .sw-step__acts { position: static; }
    :global(:where([data-vibe="paper"])) .sw-step__body::before,
    :global(:where([data-vibe="studio"])) .sw-step__body::before { content: none; }
    :global(:where([data-vibe="paper"])) .card-head,
    :global(:where([data-vibe="studio"])) .card-head { display: flex; }
    .sw-tx__more :global(.vk-button) { width: 100%; }
  }
</style>
