<script lang="ts">
  import { fetched } from "../data/fetched.svelte";
  import type { Rubric } from "../types";
  import Loading from "../ui/Loading.svelte";
  import "../ui/table.css";
  import EdgePage from "./EdgePage.svelte";
  import PageHead from "./PageHead.svelte";

  // The rubrics list (SW-Rubrics-*): Name (a link, muted note when the app cannot offer its scale),
  // Rubric id@version, Created date.
  const r = fetched<{ rubrics: Rubric[] }>(() => "/v1/rubrics");
</script>

{#if r.error}
  <EdgePage error={r.error} next="/rubrics" retry={r.reload} />
{:else if !r.data}
  <Loading />
{:else}
  <section class="sw-rubrics">
    <PageHead><h1>Rubrics</h1></PageHead>
    {#if r.data.rubrics.length === 0}
      <p class="sw-rubrics__empty">No rubrics yet. Record one with <code>superwitness rubric-add</code>.</p>
    {:else}
      <div class="sw-rubrics__panel">
        <table class="sw-table">
          <thead><tr><th>Name</th><th>Rubric</th><th>Created</th></tr></thead>
          <tbody>
            {#each r.data.rubrics as x (x.standard)}
              <tr>
                <td class="sw-wide"><a href={`/rubrics/${x.id}/${x.version}`}>{x.name}</a>{#if !x.recognised_scale}<span class="sw-muted">{" · scale not supported in the app"}</span>{/if}</td>
                <td class="mono">{x.id}@{x.version}</td>
                <td class="mono">{x.created_at.slice(0, 10)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
{/if}

<style>
  .sw-rubrics { min-width: 0; }
  .sw-rubrics__panel { padding: 16px 24px 20px; border-radius: var(--vk-radius-lg); background: var(--vk-color-surface); box-shadow: 0 var(--vk-lift, 3px) 0 var(--vk-color-ground); }
  .sw-rubrics a { color: var(--vk-color-text); font-weight: 700; text-underline-offset: 3px; }
  .sw-muted { color: var(--vk-color-muted); font-size: 0.875rem; }
  .sw-rubrics__empty { margin: 0; padding: 28px 24px; border-radius: var(--vk-radius-lg); background: var(--vk-color-raised); text-align: center; font-weight: 600; color: var(--vk-color-muted); }
  .sw-rubrics__empty code { font-family: var(--vk-font-mono); }
  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .sw-rubrics__panel { padding: 0; border-radius: 0; background: none; box-shadow: none; }
  :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .sw-rubrics__empty { border-radius: 0; background: none; text-align: left; padding: 16px 0; border-block: 1px solid var(--vk-color-line); }
  :global(:where([data-vibe="paper"])) .sw-rubrics a { font: 400 1.125rem/1.3 var(--vk-font-display); }
  @media (max-width: 759px) {
    .sw-rubrics__panel { padding: 0; background: none; box-shadow: none; }
  }
</style>
