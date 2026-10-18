<script lang="ts">
  import WarnBox from "../ui/WarnBox.svelte";
  // A tab whose source did not answer says so instead of showing nothing (the old Unavailable).
  let { source, status }: { source: string; status: string | undefined } = $props();
</script>

<WarnBox><p>unavailable: the {source} source did not answer ({status ?? "unknown"}).</p></WarnBox>
