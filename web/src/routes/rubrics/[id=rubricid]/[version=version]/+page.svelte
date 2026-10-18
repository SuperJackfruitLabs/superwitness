<script lang="ts">
  import { page } from "$app/state";
  import RubricPage from "$lib/components/RubricPage.svelte";
</script>

{#key `${page.params.id}/${page.params.version}`}
  <RubricPage id={page.params.id ?? ""} version={Number(page.params.version)} />
{/key}
