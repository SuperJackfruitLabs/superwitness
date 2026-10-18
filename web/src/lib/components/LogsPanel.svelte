<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import { getJSON } from "../api";
  import { pages } from "../data/pages.svelte";
  import { list, readableTime } from "../format";
  import type { LogLine } from "../types";
  import Empty from "../ui/Empty.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import Loading from "../ui/Loading.svelte";
  import "../ui/table.css";

  // LogsPanel is the run's log lines, 100 a page, filtered by level on the server. The level is this
  // view's own state (lost on reload, as before); a response for another level or run is dropped.
  let { api }: { api: string } = $props();

  const LEVELS = ["", "debug", "info", "warn", "error"];
  let level = $state("");
  const logs = pages<LogLine>(
    () => `${api}|${level}`,
    (cursor) =>
      getJSON<{ logs: LogLine[]; next_cursor: string | null }>(
        `${api}/logs?limit=100${level ? `&level=${level}` : ""}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`,
      ).then((p) => ({ items: list(p.logs), next: p.next_cursor ?? null })),
  );
</script>

<label class="sw-level">
  <span>Level</span>
  <select bind:value={level}>
    {#each LEVELS as l (l)}<option value={l}>{l || "all"}</option>{/each}
  </select>
</label>
{#if logs.error}
  <div class="sw-panel-fail">
    <ErrorPanel message={logs.error.message} />
    <Button variant="secondary" onclick={logs.retry}>Try again</Button>
  </div>
{:else if logs.loading}
  <Loading label="Loading logs…" />
{:else if logs.items.length === 0}
  <Empty>No log lines.</Empty>
{:else}
  <table class="sw-table">
    <thead><tr><th scope="col">Time</th><th scope="col">Level</th><th scope="col">Service</th><th scope="col">Message</th></tr></thead>
    <tbody>
      {#each logs.items as l, i (i)}
        {@const t = readableTime(l.at)}
        <tr class:err={l.level === "error"}>
          <td><time datetime={t.iso} title={t.iso}>{t.text}</time></td>
          <td><span class="sw-level-tag" data-level={l.level}>{l.level}</span></td>
          <td class="mono">{l.service}</td>
          <td class="sw-wide">{l.message}</td>
        </tr>
      {/each}
    </tbody>
  </table>
  {#if logs.moreError}<div class="sw-panel-more-err"><ErrorPanel message={logs.moreError.message} /></div>{/if}
  {#if logs.next}
    <p class="sw-panel-more"><Button variant="secondary" busy={logs.busy} onclick={logs.more}>{logs.moreError ? "Try loading more again" : "Load more"}</Button></p>
  {/if}
{/if}

<style>
  .sw-level { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; margin: 0 0 16px; font-weight: 700; }
  .sw-level select {
    min-height: 44px; min-width: 12ch; padding: 0 12px; border-radius: var(--vk-radius-sm); border: 2px solid var(--vk-color-line);
    background: var(--vk-color-surface); color: var(--vk-color-text); font: 500 16px var(--vk-font-body);
  }
  .sw-level select:focus-visible { outline: 3px solid var(--vk-color-accent); outline-offset: 2px; }
  .sw-panel-fail { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
  .sw-panel-more-err { margin-top: 12px; }
  .sw-panel-more { display: flex; justify-content: center; margin: 16px 0 0; }
  :global(:where([data-vibe="paper"])) .sw-level span { font: italic 400 18px var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-level select,
  :global(:where([data-vibe="studio"])) .sw-level select { border-radius: 0; border-color: var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-level { font-weight: 400; }
  @media (max-width: 759px) {
    .sw-level select { flex: 1; }
    .sw-panel-more :global(.vk-button) { width: 100%; }
  }
</style>
