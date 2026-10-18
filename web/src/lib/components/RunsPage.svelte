<script lang="ts">
  import { Button, Sky } from "@superjackfruit/vibekit";
  import { getJSON } from "../api";
  import { appearance } from "../appearance.svelte";
  import { pages } from "../data/pages.svelte";
  import { apiQuery, hasFilters, viewTitle, withStatus } from "../runs/filters";
  import { groupByDay } from "../runs/group";
  import { type RegistryRun, type RunPage, STATUSES } from "../types";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import Loading from "../ui/Loading.svelte";
  import StatusPill from "../ui/StatusPill.svelte";
  import EdgePage from "./EdgePage.svelte";
  import RunCard from "./RunCard.svelte";

  // The runs list (SW-Runs-*, SW-RunsStates-*): the view's title, six status chips with the first
  // page's counts, the runs by day, and Load more (also when the end scrolls near).
  let { query }: { query: string } = $props();
  const search = $derived(new URLSearchParams(query));
  const api = $derived(apiQuery(search));

  type Paged = RunPage & { items: RegistryRun[]; next: string | null };
  const list = pages<RegistryRun, Paged>(
    () => api,
    (cursor) => {
      const q = [api, cursor === null ? "" : `cursor=${encodeURIComponent(cursor)}`].filter(Boolean).join("&");
      return getJSON<RunPage>(`/v1/runs${q ? `?${q}` : ""}`).then((p) => ({ ...p, items: p.runs ?? [], next: p.next_cursor }));
    },
  );

  const counts = $derived(list.first?.counts);
  const statuses = $derived(search.getAll("status"));
  const groups = $derived(groupByDay(list.items, new Date()));
  const now = $derived.by(() => (void list.items, new Date()));

  // Scrolling near the end loads the next page. After an error nothing retries by itself: the
  // button does.
  let sentinel = $state<HTMLElement | null>(null);
  $effect(() => {
    const el = sentinel;
    if (!el || !list.next || list.moreError || list.busy || typeof IntersectionObserver === "undefined") return;
    const io = new IntersectionObserver((es) => es.some((e) => e.isIntersecting) && list.more(), { rootMargin: "400px" });
    io.observe(el);
    return () => io.disconnect();
  });
</script>

{#if list.error}
  <EdgePage error={list.error} next={"/" + (query ? `?${query}` : "")} retry={list.retry} />
{:else}
  <section class="sw-runs">
    {#if appearance.value.vibe === "daylight"}
      <div class="sw-runs__sky"><Sky phase={appearance.phase}><h1>{viewTitle(search)}</h1></Sky></div>
    {:else}
      <header class="sw-runs__head"><h1>{viewTitle(search)}</h1></header>
    {/if}
    {#if counts}
      <ul class="sw-scs" aria-label="Runs by status">
        {#each STATUSES as s (s)}
          <li>
            <a
              class="sw-sc"
              class:zero={!(counts[s] ?? 0)}
              href={withStatus(search, s)}
              aria-current={statuses.length === 1 && statuses[0] === s ? "true" : undefined}
              ><StatusPill status={s} /><span class="sw-sc__n"><span class="sw-sc__sep">{" · "}</span>{counts[s] ?? 0}</span></a
            >
          </li>
        {/each}
      </ul>
    {/if}
    {#if list.loading}
      <Loading />
    {:else if list.items.length === 0}
      <div class="sw-none">
        <p>{hasFilters(search) ? "No runs in this view." : "No runs reported yet."}</p>
        <a href="https://docs.superwitness.dev/build/run-registry/">How sources report runs</a>
      </div>
    {/if}
    {#each groups as g (g.key)}
      <section class="sw-day" aria-labelledby="day-{g.key}">
        <h2 id="day-{g.key}">{g.label}</h2>
        <ul class="sw-cards">
          {#each g.runs as r (`${r.source}/${r.external_ref}`)}
            <li><RunCard run={r} {now} /></li>
          {/each}
        </ul>
      </section>
    {/each}
    <div bind:this={sentinel}></div>
    {#if list.moreError || list.next}
      <div class="sw-more">
        {#if list.moreError}<ErrorPanel message={list.moreError.message} />{/if}
        {#if list.next}
          <Button variant="secondary" disabled={list.busy} onclick={() => list.more()}>{list.busy ? "Loading…" : "Load more"}</Button>
        {/if}
      </div>
    {/if}
  </section>
{/if}

<style>
  .sw-runs { min-width: 0; }
  h1 { margin: 0; }
  /* Daylight: the title in the sky. */
  .sw-runs__sky :global(.vk-sky) { border-radius: var(--vk-radius-lg); overflow: hidden; }
  .sw-runs__sky :global(.vk-sky__content) { padding: 28px 32px 30px; }
  .sw-runs__sky h1 { font: 800 3rem/1 var(--vk-font-display); letter-spacing: -0.02em; overflow-wrap: anywhere; }

  .sw-scs { display: flex; flex-wrap: wrap; gap: 8px; margin: 20px 0 4px; padding: 0; list-style: none; }
  .sw-sc { box-sizing: border-box;
    display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 16px 0 8px;
    border-radius: var(--vk-radius-pill); background: var(--vk-color-surface); box-shadow: inset 0 0 0 2px var(--vk-color-line);
    color: var(--vk-color-text); font-weight: 600; text-decoration: none; font-variant-numeric: tabular-nums;
  }
  .sw-sc:hover { box-shadow: inset 0 0 0 2px var(--vk-color-muted); }
  .sw-sc__n { color: var(--vk-color-muted); font-weight: 700; }
  .sw-sc.zero .sw-sc__n { color: var(--vk-color-faint); font-weight: 400; }
  .sw-sc__sep { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: pre; }
  .sw-sc[aria-current="true"] { background: var(--vk-color-accent-wash); box-shadow: inset 0 0 0 2px var(--vk-color-accent); }
  .sw-sc[aria-current="true"] .sw-sc__n { color: var(--vk-color-accent-text); }

  .sw-day h2 { margin: 28px 0 12px; font: 700 1.25rem/1.2 var(--vk-font-display); }
  .sw-cards { display: flex; flex-direction: column; gap: 12px; margin: 0; padding: 0; list-style: none; }
  .sw-cards li { min-width: 0; }

  .sw-none {
    margin-top: 16px; padding: 40px 24px; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface);
    box-shadow: 0 var(--vk-lift) 0 var(--vk-color-ground); text-align: center; display: flex; flex-direction: column; align-items: center; gap: 10px;
  }
  .sw-none p { margin: 0; font: 700 1.125rem/1.3 var(--vk-font-display); }
  .sw-none a { display: inline-flex; align-items: center; min-height: 44px; font-weight: 700; color: var(--vk-color-accent-text); text-underline-offset: 3px; }
  .sw-more { display: flex; flex-direction: column; align-items: center; gap: 10px; margin-top: 28px; }

  @media (max-width: 1099px) {
    .sw-runs__sky :global(.vk-sky__content) { padding: 22px 20px 24px; }
    .sw-runs__sky h1 { font-size: 2rem; line-height: 1.05; }
    .sw-sc { padding: 0 12px 0 8px; gap: 6px; }
    .sw-day h2 { margin: 22px 0 10px; font-size: 1.125rem; }
    .sw-cards { gap: 10px; }
    .sw-more { align-items: stretch; margin-top: 18px; }
  }

  /* Paper: a ruled masthead, chips as a ledger row of small-caps words. */
  :global(:where([data-vibe="paper"])) .sw-runs__head { padding: 22px 0 16px; border-top: 1px solid var(--vk-color-text); border-bottom: 2px solid var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-runs__head h1 { font: 500 64px/1 var(--vk-font-display); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  :global(:where([data-vibe="paper"])) .sw-scs { gap: 0; margin: 0; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-sc {
    min-height: 48px; padding: 0 18px; border-radius: 0; background: transparent; box-shadow: none; gap: 8px;
  }
  :global(:where([data-vibe="paper"])) .sw-scs li:first-child .sw-sc { padding-left: 2px; }
  :global(:where([data-vibe="paper"])) .sw-scs li + li .sw-sc { border-left: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-sc :global(.sw-pill) { box-shadow: none; padding: 0; font-size: 16px; }
  :global(:where([data-vibe="paper"])) .sw-sc:hover { text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="paper"])) .sw-sc__n, :global(:where([data-vibe="paper"])) .sw-sc.zero .sw-sc__n {
    font: 500 18px/1 var(--vk-font-display); font-variant-numeric: tabular-nums; color: var(--vk-color-text);
  }
  :global(:where([data-vibe="paper"])) .sw-sc[aria-current="true"] { background: transparent; box-shadow: inset 0 -3px 0 var(--vk-color-accent); }
  :global(:where([data-vibe="paper"])) .sw-day { margin-top: 36px; }
  :global(:where([data-vibe="paper"])) .sw-day h2 { margin: 0; padding-bottom: 8px; border-bottom: 1px solid var(--vk-color-text); font: italic 500 26px/1.2 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-cards { gap: 0; }
  :global(:where([data-vibe="paper"])) .sw-none { padding: 32px 0; border-radius: 0; background: transparent; box-shadow: none; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-none p { font: italic 400 22px/1.4 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-none a { color: var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-more { margin-top: 32px; padding-top: 24px; border-top: 3px double var(--vk-color-text); }

  /* Studio: a ruled masthead; chips as figures under their word. */
  :global(:where([data-vibe="studio"])) .sw-runs__head { display: flex; align-items: center; min-height: 64px; }
  :global(:where([data-vibe="studio"])) .sw-runs__head h1 { font: 800 28px/1.05 var(--vk-font-body); font-stretch: 80%; letter-spacing: -0.01em; overflow-wrap: anywhere; }
  :global(:where([data-vibe="studio"])) .sw-scs { gap: 0; margin: 32px 0 0; border-top: 1.5px solid var(--vk-color-rule); border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="studio"])) .sw-sc {
    flex-direction: column; align-items: flex-start; gap: 4px; min-width: 150px; padding: 12px 20px 12px 0; margin-right: 20px;
    border-radius: 0; background: transparent; box-shadow: none;
  }
  :global(:where([data-vibe="studio"])) .sw-sc :global(.sw-pill) { height: auto; padding: 0; box-shadow: none; background: none; color: var(--vk-color-text); font: 700 13px var(--vk-font-body); gap: 8px; }
  :global(:where([data-vibe="studio"])) .sw-sc :global(.sw-pill[data-tone="danger"] .sw-pill__mark) { background: var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-sc :global(.sw-pill[data-tone="danger"] .sw-pill__mark::before) { content: none; }
  :global(:where([data-vibe="studio"])) .sw-sc__n { font: 500 30px/1 var(--vk-font-mono); color: var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-sc.zero .sw-sc__n { color: var(--vk-color-muted); }
  :global(:where([data-vibe="studio"])) .sw-sc:hover :global(.sw-pill) { text-decoration: underline; text-underline-offset: 3px; }
  :global(:where([data-vibe="studio"])) .sw-sc[aria-current="true"] { background: transparent; box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-sc[aria-current="true"] :global(.sw-pill) { text-decoration: underline; text-decoration-thickness: 3px; text-underline-offset: 5px; }
  :global(:where([data-vibe="studio"])) .sw-runs { counter-reset: sw-day; }
  :global(:where([data-vibe="studio"])) .sw-day { margin-top: 32px; counter-increment: sw-day; }
  :global(:where([data-vibe="studio"])) .sw-day h2::before { content: counter(sw-day, decimal-leading-zero); margin-right: 14px; font: 500 12px var(--vk-font-mono); color: var(--vk-color-muted); }
  :global(:where([data-vibe="studio"])) .sw-day h2 { margin: 0; padding: 10px 0 8px; border-bottom: 1.5px solid var(--vk-color-rule); font: 800 16px/1.2 var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-cards { gap: 0; }
  :global(:where([data-vibe="studio"])) .sw-none { align-items: flex-start; text-align: left; padding: 20px 0; border-radius: 0; background: transparent; box-shadow: none; border-top: 1.5px solid var(--vk-color-rule); }
  :global(:where([data-vibe="studio"])) .sw-none p { font: 800 18px/1.3 var(--vk-font-body); }
  :global(:where([data-vibe="studio"])) .sw-none a { color: var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-more { margin-top: 8px; padding-top: 8px; }

  /* Quiet: plain words. */
  :global(:where([data-vibe="quiet"])) .sw-runs__head h1 { margin: var(--vk-space-6) 0 var(--vk-space-3); font: 700 2.25rem/1.2 var(--vk-font-display); overflow-wrap: anywhere; }
  :global(:where([data-vibe="quiet"])) .sw-scs { gap: 4px 22px; margin: 0 0 var(--vk-space-4); }
  :global(:where([data-vibe="quiet"])) .sw-sc { padding: 0; gap: 0; border-radius: 0; background: transparent; box-shadow: none; font-weight: 400; text-decoration: underline; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-sc :global(.sw-pill) { border: 0; padding: 0; background: none; color: inherit; font-weight: 400; line-height: inherit; }
  :global(:where([data-vibe="quiet"])) .sw-sc__sep { position: static; width: auto; height: auto; clip-path: none; }
  :global(:where([data-vibe="quiet"])) .sw-sc__n, :global(:where([data-vibe="quiet"])) .sw-sc.zero .sw-sc__n { color: inherit; font-weight: 400; }
  :global(:where([data-vibe="quiet"])) .sw-sc.zero { color: var(--vk-color-muted); }
  :global(:where([data-vibe="quiet"])) .sw-sc[aria-current="true"] { font-weight: 700; text-decoration: none; box-shadow: inset 0 -3px 0 var(--vk-color-accent); }
  :global(:where([data-vibe="quiet"])) .sw-sc[aria-current="true"] :global(.sw-pill) { font-weight: 700; }
  :global(:where([data-vibe="quiet"])) .sw-day h2 { margin: var(--vk-space-5) 0 var(--vk-space-2); font: 700 1.375rem/1.3 var(--vk-font-display); }
  :global(:where([data-vibe="quiet"])) .sw-cards { gap: 0; border-bottom: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="quiet"])) .sw-none { align-items: flex-start; text-align: left; padding: 0; margin-top: var(--vk-space-4); border-radius: 0; background: transparent; box-shadow: none; gap: 0; }
  :global(:where([data-vibe="quiet"])) .sw-none p { font: 400 1.125rem/1.6 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-none a { color: var(--vk-color-text); font-weight: 400; text-underline-offset: 4px; }
  :global(:where([data-vibe="quiet"])) .sw-more { align-items: flex-start; margin-top: var(--vk-space-5); }
  @media (max-width: 1099px) {
    :global(:where([data-vibe="paper"])) .sw-runs__head { border-top: 0; padding-top: 12px; padding-bottom: 12px; }
    :global(:where([data-vibe="paper"])) .sw-runs__head h1 { font-size: 44px; }
    :global(:where([data-vibe="paper"])) .sw-day { margin-top: 26px; }
    :global(:where([data-vibe="paper"])) .sw-day h2 { font-size: 22px; padding-bottom: 6px; }
    :global(:where([data-vibe="paper"])) .sw-scs { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
    :global(:where([data-vibe="paper"])) .sw-sc { padding: 0 6px 0 10px; border-bottom: 1px solid var(--vk-color-line); }
    :global(:where([data-vibe="paper"])) .sw-scs li:nth-child(n + 4) .sw-sc { border-bottom: 0; }
    :global(:where([data-vibe="paper"])) .sw-scs li:nth-child(3n + 1) .sw-sc { border-left: 0; padding-left: 2px; }
    :global(:where([data-vibe="studio"])) .sw-runs__head h1 { font: 900 44px/0.95 var(--vk-font-body); font-stretch: 75%; }
    :global(:where([data-vibe="studio"])) .sw-scs { margin-top: 16px; border-bottom: 0; }
    :global(:where([data-vibe="studio"])) .sw-sc { flex-direction: row; align-items: center; min-width: 0; padding: 0 14px 0 0; margin: 0; gap: 8px; }
    :global(:where([data-vibe="studio"])) .sw-sc__n { font: 500 13.5px var(--vk-font-mono); }
    :global(:where([data-vibe="quiet"])) .sw-more { align-items: stretch; }
    :global(:where([data-vibe="quiet"])) .sw-runs__head h1 { font-size: 1.875rem; margin: var(--vk-space-5) 0 var(--vk-space-2); }
  }
</style>
