<script lang="ts">
  import { display, list } from "../format";
  import type { Doc } from "../types";
  import Empty from "../ui/Empty.svelte";
  import StatusPill from "../ui/StatusPill.svelte";
  import "../ui/table.css";

  // AttemptsPanel is each attempt with its configuration fingerprint. Missing values are the word
  // "unknown"; the 71-character digest wraps inside its cell.
  let { doc }: { doc: Doc } = $props();
  const attempts = $derived(list<Doc>(doc.attempts));
</script>

{#snippet value(v: unknown, mono = false)}
  {#if v === null || v === undefined || v === "unknown"}<span class="sw-word">unknown</span>{:else if mono}<span class="mono">{display(v)}</span>{:else}{display(v)}{/if}
{/snippet}

{#if attempts.length === 0}
  <Empty>No attempts.</Empty>
{:else}
  <table class="sw-table">
    <thead><tr><th scope="col">Attempt</th><th scope="col">State</th><th scope="col">Configuration fingerprint</th><th scope="col">Harness</th><th scope="col">Model</th></tr></thead>
    <tbody>
      {#each attempts as a, i (a.id ?? i)}
        <tr>
          <td class="mono">{display(a.id)}</td>
          <td>{#if a.state === undefined || a.state === null}<span class="sw-word">unknown</span>{:else}<StatusPill status={a.state} />{/if}</td>
          <td data-label="Configuration fingerprint">{#if a.fingerprint?.digest && a.fingerprint.digest !== "unknown"}<span class="sw-digest">{a.fingerprint.digest}</span>{:else}<span class="sw-word">unknown</span>{/if}</td>
          <td data-label="Harness"><span>{@render value(a.fingerprint?.harness)} {@render value(a.fingerprint?.harness_version)}</span></td>
          <td data-label="Model">{@render value(a.fingerprint?.model, true)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}
