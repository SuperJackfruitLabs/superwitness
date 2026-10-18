<script lang="ts" module>
  // How often a "Can't reach the database" page asks again on its own.
  export const DATABASE_RETRY_MS = 30_000;
</script>

<script lang="ts">
  import { Button } from "@superjackfruit/vibekit";
  import type { ApiError } from "../api";
  import { signOut } from "../signout.svelte";
  import Panel from "../ui/Panel.svelte";
  import InShell from "../ui/InShell.svelte";
  import ErrorPanel from "../ui/ErrorPanel.svelte";

  // EdgePage turns an API refusal into the page it calls for (the old ErrorView). next is where a
  // sign-in returns to; retry, when given, reloads what failed; assign is for tests.
  let {
    error,
    next = "/",
    retry,
    assign,
  }: { error: ApiError; next?: string; retry?: () => void; assign?: (to: string) => void } = $props();

  const out = signOut((to) => (assign ? assign(to) : window.location.assign(to)));
  const kind = $derived(
    error.status === 401
      ? "signed_out"
      : ["not_authorised", "store_unavailable", "run_not_found", "rubric_not_found", "rate_limited", "principal_unresolved", "origin_mismatch"].includes(error.code)
        ? error.code
        : "other",
  );

  $effect(() => {
    if (kind !== "store_unavailable" || !retry) return;
    const r = retry;
    const t = setInterval(r, DATABASE_RETRY_MS);
    return () => clearInterval(t);
  });
</script>

{#if kind === "run_not_found"}
  <InShell>
    <h1>Run not found</h1>
    <p>Neither superpipeline nor AgentPod knows this run.</p>
    <Button variant="secondary" href="/">All runs</Button>
  </InShell>
{:else if kind === "rubric_not_found"}
  <InShell>
    <h1>Rubric not found</h1>
    <Button variant="secondary" href="/rubrics">Rubrics</Button>
  </InShell>
{:else if out.state.kind === "lingering"}
  <Panel>
    <h1>Signed out</h1>
    <p class="sw-lead">
      You are signed out in this browser. The database did not answer, so the server-side session may outlive this
      sign-out. On a shared computer, sign in again later and sign out once more.
    </p>
    <p class="sw-act"><Button href="/" data-sveltekit-reload>Continue</Button></p>
  </Panel>
{:else}
  <Panel>
    {#if kind === "signed_out"}
      <h1>superwitness</h1>
      <p class="sw-lead">Sign in to browse runs and record verdicts.</p>
      <p class="sw-act"><Button href={`/auth/login?next=${encodeURIComponent(next)}`} data-sveltekit-reload>Sign in</Button></p>
    {:else if kind === "not_authorised"}
      <h1>Not authorised</h1>
      <p class="sw-lead">This account is not allowed to use this superwitness. Ask its operator to add you.</p>
      <p class="sw-act"><Button variant="secondary" onclick={out.run} disabled={out.state.kind === "busy"}>Sign out</Button></p>
      {#if out.state.kind === "failed"}
        <p class="sw-failed" role="alert">
          Sign out failed — try again. {out.state.reason}
          <Button variant="text" onclick={out.run}>Try again</Button>
        </p>
      {/if}
    {:else if kind === "store_unavailable"}
      <h1>Can’t reach the database</h1>
      <p class="sw-lead">superwitness is running but its database is not answering. This page will work again when it does.</p>
      {#if retry}<p class="sw-act"><Button onclick={retry}>Try again</Button></p>{/if}
    {:else if kind === "rate_limited"}
      <h1>Too many requests</h1>
      <ErrorPanel message={`Too many requests — try again${error.retryAfter !== null ? ` in ${error.retryAfter} s` : " shortly"}.`} />
    {:else if kind === "principal_unresolved"}
      <h1>Can’t confirm who you are</h1>
      <ErrorPanel message="AgentPod can’t be reached to confirm who you are. Try again shortly." />
    {:else if kind === "origin_mismatch"}
      <h1>Request refused</h1>
      <ErrorPanel message="This request didn’t come from the app. Reload the page and try again." />
    {:else}
      <h1>Something went wrong</h1>
      <ErrorPanel message={error.message} code={/^HTTP \d+$/.test(error.message)} />
    {/if}
  </Panel>
{/if}

<style>
  .sw-failed { color: var(--sw-danger-text); font-weight: 600; }
</style>
