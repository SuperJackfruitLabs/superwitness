<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import type { ApiError } from "../api";
  import Empty from "../ui/Empty.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";
  import WarnBox from "../ui/WarnBox.svelte";
  // TranscriptRefusal says why session content is not shown; the rest of the page keeps working.
  // 403 transcripts_forbidden is a muted line, 503 or no network an amber box with Try again,
  // anything else the refusal as it came.
  let { error, onretry }: { error: ApiError; onretry: () => void } = $props();
</script>

{#if error.status === 403 && error.code === "transcripts_forbidden"}
  <Empty>Transcripts aren't available to this account</Empty>
{:else if error.status === 503 || error.status === 0}
  <WarnBox role="alert">
    <p>Transcript unavailable, try again</p>
    {#snippet actions()}<Button variant="secondary" onclick={onretry}>Try again</Button>{/snippet}
  </WarnBox>
{:else}
  <ErrorPanel message={error.message} />
{/if}
