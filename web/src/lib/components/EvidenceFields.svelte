<script lang="ts">
  import EvidenceFields from "./EvidenceFields.svelte";
  let { value }: { value: unknown } = $props();
  const label = (key: string) => key.replaceAll("_", " ");
</script>

{#if Array.isArray(value)}
  {#if value.length === 0}<p class="empty">None recorded in this snapshot.</p>{/if}
  <ol>
    {#each value as item, i}
      <li>
        {#if item && typeof item === "object"}
          <details><summary>{item.id ?? item.session_id ?? item.source_key ?? item.path ?? `Record ${i + 1}`} {item.state ?? item.status ?? item.disposition ?? ""}</summary><EvidenceFields value={item} /></details>
        {:else}<span>{item ?? "unknown"}</span>{/if}
      </li>
    {/each}
  </ol>
{:else if value && typeof value === "object"}
  <dl>
    {#each Object.entries(value) as [key, field]}
      <div><dt>{label(key)}</dt><dd>
        {#if field && typeof field === "object"}<EvidenceFields value={field} />
        {:else}{field === null || field === undefined ? "unknown" : String(field)}{/if}
      </dd></div>
    {/each}
  </dl>
{:else}<span>{value ?? "unknown"}</span>{/if}

<style>
  dl { margin: 8px 0; } dl > div { display: grid; grid-template-columns: minmax(110px, 1fr) minmax(0, 3fr); gap: 12px; padding: 5px 0; border-bottom: 1px solid var(--vk-color-line); }
  dt { color: var(--vk-color-muted); text-transform: capitalize; } dd { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  ol { padding-left: 20px; margin: 8px 0; } li { padding: 4px 0; } summary { cursor: pointer; overflow-wrap: anywhere; padding: 8px 0; } .empty { color: var(--vk-color-muted); }
  @media (max-width: 599px) { dl > div { grid-template-columns: 1fr; gap: 4px; } }
</style>
