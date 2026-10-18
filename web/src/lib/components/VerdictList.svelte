<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import { display, list, readableTime, verdictSummary } from "../format";
  import type { History } from "../run/history.svelte";
  import { runURL, seqParam } from "../run/transcript";
  import type { Doc, HistoryVerdict, Me } from "../types";
  import Empty from "../ui/Empty.svelte";
  import Loading from "../ui/Loading.svelte";
  import WarnBox from "../ui/WarnBox.svelte";

  // VerdictList shows the gates (read-only, from superpipeline) and the recorded verdicts as history:
  // a superseded verdict is struck through and links to its successor. Revise is offered on the
  // viewer's own latest rubric verdicts. A subject whose history could not be read says so; it is
  // never shown as empty.
  let { gates, history, me, onrevise, attempts = [], page = "" }: {
    gates: Doc[]; history: History; me: Me | null; onrevise: (v: HistoryVerdict) => void; attempts?: Doc[]; page?: string;
  } = $props();

  const verdicts = $derived(history.verdicts);
  const failures = $derived(history.failures);

  // A gate's decision in words: "no decision yet" while it has none.
  function gateValue(v: unknown): string | null {
    if (!v || typeof v !== "object") return null;
    const d = (v as { decision?: unknown }).decision;
    if ("decision" in v && (d === null || d === undefined)) return null;
    return verdictSummary(v as Record<string, unknown>);
  }
  const tone = (s: string) => (["rejected", "fail", "failed"].includes(s) ? "bad" : ["approved", "pass", "passed"].includes(s) ? "good" : "");

  function links(v: HistoryVerdict) {
    const out: { href: string; label: string }[] = [];
    for (const r of list<unknown>(v.evidence_refs)) {
      const ref = r as { session_id?: unknown; seq_from?: unknown; seq_to?: unknown };
      if (typeof r !== "object" || r === null || typeof ref.seq_from !== "number" || typeof ref.seq_to !== "number") continue;
      const from = ref.seq_from;
      const inSession = attempts.filter((x) => x.session_id === ref.session_id);
      const a = inSession.find((x) => x.seq_from <= from && (x.seq_to === null || from <= x.seq_to)) ?? inSession[0];
      if (!a) continue;
      out.push({ href: runURL(page, { tab: "transcript", attempt: a.id, seq: seqParam(ref.seq_from, ref.seq_to) }), label: `transcript ${ref.seq_from}–${ref.seq_to} ↗` });
    }
    return out;
  }
</script>

{#if failures.length > 0}
  <div class="sw-vfail">
    <WarnBox role="alert">
      {#each failures as f (f.subject)}<p>unavailable: the verdict history for {f.subject} could not be read ({f.message}).</p>{/each}
      {#snippet actions()}<Button variant="secondary" onclick={history.retry}>Try again</Button>{/snippet}
    </WarnBox>
  </div>
{/if}
{#if history.loading && verdicts.length === 0 && gates.length === 0 && failures.length === 0}
  <Loading label="Loading verdicts…" />
{:else if verdicts.length === 0 && gates.length === 0}
  {#if failures.length === 0}<Empty>No verdicts yet.</Empty>{/if}
{:else}
  <ul class="verdicts">
    {#each gates as g (g.id)}
      {@const value = gateValue(g.value)}
      <li class="verdict gate">
        <div class="row">
          <strong>Gate</strong>
          <span class="mono">{display(g.standard)}</span>
          <span class="sw-gstatus" data-status={display(g.status)}>{display(g.status)}</span>
          {#if value === null}<span class="sw-word">no decision yet</span>{:else}<span class="sw-vtag" data-tone={tone(value)}>{value}</span>{/if}
          {#if g.judge && g.judge !== "unknown"}<span class="mono muted">{display(g.judge)}</span>{/if}
        </div>
      </li>
    {/each}
    {#each verdicts as v (v.id)}
      {@const t = readableTime(v.created_at)}
      {@const own = !v.superseded_by && !!me && v.judge === me.principal && v.standard.startsWith("rubric:")}
      <li id={v.id} class="verdict" class:superseded={!!v.superseded_by}>
        <div class="row">
          <span class="value">{#if v.superseded_by}<s>{verdictSummary(v.value)}</s>{:else}<strong>{verdictSummary(v.value)}</strong>{/if}</span>
          <span class="mono">{v.standard}</span>
          <span class="muted">{v.judge} ({v.judge_kind})</span>
          <time class="mono muted" datetime={t.iso} title={t.iso}>{t.text}</time>
          {#if v.subject_kind === "attempt"}<span class="mono">{v.subject_ref}</span>{/if}
          {#if v.superseded_by}<a href={`#${v.superseded_by}`}>superseded by {v.superseded_by}</a>{/if}
          {#each links(v) as l, i (i)}<a href={l.href}>{l.label}</a>{/each}
          {#if own}<span class="sw-revise"><Button variant="secondary" onclick={() => onrevise(v)}>Revise</Button></span>{/if}
        </div>
        {#if v.comment}<p class="comment">{v.comment}</p>{/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  .sw-vfail { margin-bottom: 14px; }
  .verdicts { display: flex; flex-direction: column; gap: 12px; margin: 0; padding: 0; list-style: none; min-width: 0; }
  .verdict { min-width: 0; padding: 14px 20px; border-radius: var(--vk-radius-md); background: var(--vk-color-surface); box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); overflow-wrap: anywhere; scroll-margin: 96px; }
  .verdict:target { outline: 3px solid var(--vk-color-accent); }
  .verdict.superseded { background: var(--vk-color-raised); box-shadow: none; color: var(--vk-color-muted); }
  .row { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 12px; min-width: 0; }
  .row strong { font: 800 1.125rem/1.2 var(--vk-font-display); }
  .gate .row strong { font-size: 1rem; }
  .mono { font: 500 0.875rem/1.4 var(--vk-font-mono); }
  .muted { color: var(--vk-color-muted); }
  s { font: 800 1.125rem/1.2 var(--vk-font-display); color: var(--vk-color-muted); }
  .sw-gstatus { font-weight: 700; }
  .sw-gstatus[data-status="cancelled"] { padding: 1px 9px; border-radius: var(--vk-radius-pill); background: var(--vk-color-raised); color: var(--vk-color-muted); box-shadow: inset 0 0 0 1px var(--vk-color-line); font-size: 0.8125rem; }
  .sw-word { color: var(--vk-color-muted); font-style: italic; }
  .sw-vtag { padding: 1px 10px; border-radius: var(--vk-radius-pill); background: var(--vk-color-raised); font-size: 0.8125rem; font-weight: 700; }
  .sw-vtag[data-tone="bad"] { background: var(--sw-danger-wash); color: var(--sw-danger-text); box-shadow: inset 0 0 0 1px var(--sw-danger); }
  .sw-vtag[data-tone="good"] { background: var(--vk-color-accent-wash); color: var(--vk-color-accent-text); box-shadow: inset 0 0 0 1px var(--vk-color-accent); }
  a { display: inline-flex; align-items: center; min-height: 44px; font-weight: 600; color: inherit; text-underline-offset: 3px; }
  a:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  .sw-revise { margin-left: auto; }
  .comment { margin: 8px 0 0; color: var(--vk-color-text); white-space: pre-wrap; }

  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .verdicts { gap: 0; border-top: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .verdict { padding: 14px 4px; border-radius: 0; background: none; box-shadow: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .row strong, :global(:where([data-vibe="paper"])) s { font: 500 22px/1.2 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .gate .row strong { font: 600 13px var(--vk-font-body); letter-spacing: 0.1em; text-transform: uppercase; }
  :global(:where([data-vibe="paper"])) .sw-word { font-family: var(--vk-font-display); font-size: 1.0625rem; }
  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .sw-vtag,
  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .sw-gstatus[data-status="cancelled"] { border-radius: 0; }
  :global(:where([data-vibe="studio"])) .row strong, :global(:where([data-vibe="studio"])) s { font: 800 18px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-vtag[data-tone="bad"] { background: var(--vk-color-text); color: var(--vk-color-bg); box-shadow: none; font-family: var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-word { font-family: var(--vk-font-serif); }
  :global(:where([data-vibe="quiet"])) .row strong, :global(:where([data-vibe="quiet"])) s { font: 700 1.125rem var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-vtag { background: none; color: var(--vk-color-text); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .muted { color: var(--vk-color-text); }
  @media (max-width: 759px) {
    .verdict { padding: 12px 14px; }
    .sw-revise { margin-left: 0; width: 100%; }
    .sw-revise :global(.vk-button) { width: 100%; }
  }
</style>
