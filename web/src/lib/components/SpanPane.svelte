<script lang="ts">
  import { fetched } from "../data/fetched.svelte";
  import { list, readableTime } from "../format";
  import { navigate } from "../nav";
  import { logsOf, runURL, spanRange, spanStatus } from "../run/transcript";
  import type { Doc, LogLine, Span } from "../types";
  import Empty from "../ui/Empty.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import Inspector from "../ui/Inspector.svelte";
  import KeyValue from "../ui/KeyValue.svelte";
  import Loading from "../ui/Loading.svelte";
  import SpanSession from "./SpanSession.svelte";

  // SpanPane is one span's details, opened from the waterfall (SW-SpanPane-*): its facts, its
  // attributes, its log lines, its session content, and Cite. It reads session content only now.
  let { span, api, page, attempts, cited, canCite, oncite }: {
    span: Span; api: string; page: string; attempts: Doc[]; cited: boolean; canCite: boolean; oncite: () => void;
  } = $props();

  const range = $derived(spanRange(span, attempts));
  const attempt = $derived(range ? attempts.find((a) => a.id === range.attempt) : undefined);
  const logs = fetched<{ logs: LogLine[] }>(() => `${api}/logs?limit=500`);
  const lines = $derived(logsOf(list(logs.data?.logs), span));
  const status = $derived(spanStatus(span));
  const start = $derived(readableTime(span.start));
  const keys = $derived(Object.keys(span.attributes).sort());
</script>

{#snippet startValue()}<time datetime={start.iso} title={start.iso}>{start.text}</time>{/snippet}
{#snippet statusValue()}<span class="sw-spanstatus" class:err={status === "ERROR"}>{status}</span>{/snippet}

<Inspector label="Span details" onclose={() => navigate(runURL(page, {}))}>
  {#snippet head()}
    <h2>{span.name}</h2>
    <p class="sw-spanid">{span.span_id}</p>
  {/snippet}
  <KeyValue
    items={[
      { term: "Service", value: span.service, wide: true },
      { term: "Start", value: startValue, wide: true },
      { term: "Duration", value: `${Math.round(span.duration_ms)} ms` },
      { term: "Status", value: statusValue },
    ]}
  />
  <section>
    <h3>Attributes</h3>
    <table class="sw-attrs">
      <tbody>
        {#each keys as k (k)}
          <tr><th scope="row">{k}</th><td>{span.attributes[k]}</td></tr>
        {/each}
      </tbody>
    </table>
  </section>
  <section>
    <h3>Logs</h3>
    {#if logs.error}
      <ErrorPanel message={logs.error.message} />
    {:else if lines.length > 0}
      <ul class="sw-spanlogs">
        {#each lines as l, i (i)}
          {@const t = readableTime(l.at)}
          <li class:err={l.level === "error"}><b>{l.level}</b><span class="sw-spanlogs__m">{l.message}</span><time datetime={t.iso} title={t.iso}>{t.text}</time></li>
        {/each}
      </ul>
    {:else if logs.loading}
      <Loading label="Loading logs…" />
    {:else}
      <Empty>No log lines in this span.</Empty>
    {/if}
  </section>
  {#if range}
    <SpanSession {api} {page} {range} spanName={span.name} running={attempt?.seq_to === null} />
  {:else}
    <Empty>no session content for this span</Empty>
  {/if}
  {#snippet foot()}
    <button type="button" class="sw-cite" aria-pressed={cited ? "true" : "false"} disabled={!cited && !canCite} onclick={oncite}>{cited ? "Cited" : "Cite"}</button>
  {/snippet}
</Inspector>

<style>
  h2 { margin: 0; font: 800 1.5rem/1.2 var(--vk-font-display); overflow-wrap: anywhere; }
  .sw-spanid { margin: 2px 0 0; font: 500 0.75rem/1.4 var(--vk-font-mono); color: var(--vk-color-muted); overflow-wrap: anywhere; }
  h3 { margin: 0 0 8px; font: 700 1rem/1.2 var(--vk-font-display); }
  section { min-width: 0; }
  .sw-spanstatus { font-weight: 700; }
  .sw-spanstatus.err { display: inline-block; padding: 0 9px; border-radius: var(--vk-radius-pill); background: var(--sw-danger-wash); color: var(--sw-danger-text); box-shadow: inset 0 0 0 1px var(--sw-danger); font-size: 0.875rem; }
  .sw-attrs { width: 100%; table-layout: fixed; border-collapse: collapse; font: 500 0.8125rem/1.4 var(--vk-font-mono); }
  .sw-attrs th, .sw-attrs td { padding: 6px 10px; text-align: left; vertical-align: top; border-bottom: 1px solid var(--vk-color-line); overflow-wrap: anywhere; }
  .sw-attrs th { width: 44%; font-weight: 500; color: var(--vk-color-muted); }
  .sw-spanlogs { display: flex; flex-direction: column; gap: 6px; margin: 0; padding: 0; list-style: none; }
  .sw-spanlogs li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 10px; padding: 10px 12px; border-radius: var(--vk-radius-sm); background: var(--vk-color-raised); font-size: var(--vk-text-sm); overflow-wrap: anywhere; }
  .sw-spanlogs li.err { background: var(--sw-danger-wash); color: var(--sw-danger-text); }
  .sw-spanlogs b { font-weight: 800; }
  .sw-spanlogs__m { flex: 1 1 16ch; min-width: 0; }
  .sw-spanlogs time { white-space: nowrap; color: var(--vk-color-muted); }
  .sw-cite {
    display: inline-flex; align-items: center; justify-content: center; min-height: 44px; min-width: 96px; padding: 0 20px; border-radius: var(--vk-radius-pill);
    border: 2px solid var(--vk-color-accent); background: var(--vk-color-surface); color: var(--vk-color-text); font: 700 1rem/1 var(--vk-font-body); cursor: pointer;
  }
  .sw-cite[aria-pressed="true"] { background: var(--vk-color-accent); color: var(--vk-color-accent-fg); }
  .sw-cite:disabled { opacity: 0.5; cursor: not-allowed; }
  .sw-cite:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }

  :global(:where([data-vibe="paper"])) h2 { font: 500 34px/1.05 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) h3 { font: italic 500 22px/1.2 var(--vk-font-display); padding-bottom: 6px; border-bottom: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-spanstatus.err { padding: 0; background: none; box-shadow: none; font-size: inherit; }
  :global(:where([data-vibe="paper"])) .sw-spanlogs li { padding: 6px 0; border-radius: 0; background: none; border-bottom: 1px solid var(--vk-color-line); font-family: var(--vk-font-mono); }
  :global(:where([data-vibe="paper"])) .sw-spanlogs li.err { background: none; }
  :global(:where([data-vibe="paper"])) .sw-spanlogs b { font: 600 13px/1 var(--vk-font-body); font-variant-caps: all-small-caps; letter-spacing: 0.08em; }
  :global(:where([data-vibe="paper"])) .sw-cite { border-radius: var(--vk-radius-sm); border-color: var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-cite[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="studio"])) h2 { font: 800 30px/1.05 var(--vk-font-body); font-stretch: 82%; }
  :global(:where([data-vibe="studio"])) h3 { font: 800 15px var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-spanstatus.err { padding: 1px 6px; border-radius: 0; background: var(--vk-color-text); color: var(--vk-color-bg); box-shadow: none; font: 700 12px var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-attrs { font-size: 13.5px; }
  :global(:where([data-vibe="studio"])) .sw-spanlogs li { border-radius: 0; background: none; padding: 4px 0; font-family: var(--vk-font-mono); }
  :global(:where([data-vibe="studio"])) .sw-spanlogs li.err { color: var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-spanlogs li.err b { padding: 1px 5px; background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="studio"])) .sw-cite { border-radius: 0; border-color: var(--vk-color-text); font-weight: 800; font-size: 14px; }
  :global(:where([data-vibe="studio"])) .sw-cite[aria-pressed="true"] { background: var(--vk-color-text); color: var(--vk-color-bg); }
  :global(:where([data-vibe="quiet"])) h2 { font: 700 1.375rem/1.3 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) h3 { font: 700 1.125rem/1.3 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-spanid { font-size: 0.9375rem; }
  :global(:where([data-vibe="quiet"])) .sw-spanstatus.err { padding: 0; background: none; box-shadow: none; color: var(--vk-color-text); font-size: inherit; }
  :global(:where([data-vibe="quiet"])) .sw-attrs { font-size: 1rem; }
  :global(:where([data-vibe="quiet"])) .sw-spanlogs li { padding: 2px 0 2px 14px; border-radius: 0; background: none; border-left: 4px solid var(--vk-color-text); font-size: 1rem; color: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-cite { border-radius: var(--vk-radius-sm); }

  @media (max-width: 759px) {
    .sw-attrs, .sw-attrs tbody, .sw-attrs tr, .sw-attrs th, .sw-attrs td { display: block; width: auto; }
    .sw-attrs tr { padding: 6px 0; border-bottom: 1px solid var(--vk-color-line); }
    .sw-attrs th, .sw-attrs td { padding: 0; border: 0; }
    .sw-attrs td { color: var(--vk-color-text); font-weight: 700; }
    .sw-cite { flex: 1; }
  }
</style>
