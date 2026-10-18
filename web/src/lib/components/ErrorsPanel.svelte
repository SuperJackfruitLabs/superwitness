<script lang="ts">
  import { display, list, readableTime } from "../format";
  import type { Doc } from "../types";
  import Empty from "../ui/Empty.svelte";
  import "../ui/table.css";

  // ErrorsPanel is the run document's errors (not paged): time, service, message.
  let { doc }: { doc: Doc } = $props();
  const errors = $derived(list<Doc>(doc.errors));
</script>

{#if errors.length === 0}
  <Empty>No errors.</Empty>
{:else}
  <table class="sw-table">
    <thead><tr><th scope="col">Time</th><th scope="col">Service</th><th scope="col">Message</th></tr></thead>
    <tbody>
      {#each errors as e, i (i)}
        {@const t = typeof e.at === "string" ? readableTime(e.at) : null}
        <tr class="err">
          <td>{#if t}<time datetime={t.iso} title={t.iso}>{t.text}</time>{:else}<span class="sw-word">unknown</span>{/if}</td>
          <td class="mono">{display(e.service)}</td>
          <td class="sw-wide">{display(e.message)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}
