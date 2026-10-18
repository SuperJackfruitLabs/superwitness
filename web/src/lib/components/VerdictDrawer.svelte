<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { Button } from "@superjackfruit/vibekit";
  import { ApiError, postJSON } from "../api";
  import { fetched } from "../data/fetched.svelte";
  import { MAX_EVIDENCE } from "../evidence.svelte";
  import { list } from "../format";
  import type { Doc, EvidenceRef, HistoryVerdict, Rubric } from "../types";
  import { citedEvidence, evidenceSummary, MAX_COMMENT } from "../verdict/drawer";
  import { refusalText } from "../verdict/messages";
  import { initialInput, type Input, supported, valueOf } from "../verdict/scale";
  import ScaleInput from "./ScaleInput.svelte";

  // VerdictDrawer records a verdict on the run, or on one of its attempts, against a rubric
  // (SW-RunVerdicts-*, SW-DrawerVariants-*, SW-Phone-Drawer-*). The idempotency key is made when
  // the drawer opens, so a double click or a retry records one verdict. The evidence is the run
  // view's: ticked spans and cited transcript steps (and, revising, the earlier verdict's). A 401
  // while saving ends the session (api.ts tells session.svelte.ts): the layout swaps the page for
  // the sign-in page carrying the path and query, and nothing is recorded or half-kept.
  let { doc, evidence: ticked, revising, onclose, onrecorded }: {
    doc: Doc;
    evidence: EvidenceRef[];
    revising?: HistoryVerdict;
    onclose: () => void;
    onrecorded: () => void;
  } = $props();

  const rubrics = fetched<{ rubrics: Rubric[] }>(() => "/v1/rubrics");
  const all = $derived(rubrics.data?.rubrics ?? []);
  const evidence = $derived(citedEvidence(ticked, revising));
  const kept = $derived(citedEvidence([], revising).length);
  const key = crypto.randomUUID();
  const runRef = $derived(String(doc.run?.ref ?? ""));
  const attempts = $derived(list<Doc>(doc.attempts).map((a) => String(a.id)));
  const titleId = $props.id();

  let subject = $state(untrack(() => (revising ? `${revising.subject_kind}|${revising.subject_ref}` : `run|${doc.run?.ref}`)));
  let standard = $state(untrack(() => revising?.standard ?? ""));
  let comment = $state(untrack(() => revising?.comment ?? ""));
  let input = $state<Input | null>(null);
  let sending = $state(false);
  let refusal = $state<string | null>(null);

  const usable = $derived(all.filter((r) => supported(r.recognised_scale)));
  const rubric = $derived(all.find((r) => r.standard === standard));
  const scale = $derived(supported(rubric?.recognised_scale) ? rubric.recognised_scale : null);
  // A new rubric (or its scale arriving) starts a fresh input; a revision starts from its value.
  const inputKey = $derived(`${standard}|${scale?.kind ?? ""}`);
  $effect(() => {
    void inputKey;
    untrack(() => {
      input = scale ? initialInput(scale, revising?.standard === standard ? revising.value : undefined) : null;
    });
  });

  const value = $derived(scale && input ? valueOf(scale, input) : null);
  const commentLen = $derived([...comment].length); // the server counts characters, not UTF-16 units
  const tooMuch = $derived(evidence.length > MAX_EVIDENCE);
  const ready = $derived(value !== null && commentLen <= MAX_COMMENT && !tooMuch && !sending);

  // While a save is in flight the drawer cannot be closed: the verdict may still be recorded, and
  // the person should see the outcome.
  function close() {
    if (!sending) onclose();
  }

  // Focus moves into the drawer on open, stays inside while it is open, and returns to whatever
  // opened it on close.
  let panel = $state<HTMLElement | null>(null);
  onMount(() => {
    const opener = document.activeElement as HTMLElement | null;
    panel?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") return close();
      if (e.key !== "Tab" || !panel) return;
      const f = [...panel.querySelectorAll<HTMLElement>("a[href], button, input, select, textarea, [tabindex]")].filter(
        (x) => !(x as HTMLButtonElement).disabled && x.tabIndex >= 0,
      );
      if (f.length === 0) return e.preventDefault();
      const first = f[0];
      const last = f[f.length - 1];
      const at = document.activeElement;
      if (!panel.contains(at) || (e.shiftKey && (at === first || at === panel))) {
        e.preventDefault();
        (e.shiftKey ? last : first).focus();
      } else if (!e.shiftKey && at === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      opener?.focus?.();
    };
  });

  async function send() {
    if (!ready || value === null) return;
    const cut = subject.indexOf("|");
    sending = true;
    refusal = null;
    try {
      await postJSON("/v1/verdicts", {
        idempotency_key: key,
        subject_kind: subject.slice(0, cut),
        subject_ref: subject.slice(cut + 1),
        standard,
        value,
        ...(comment ? { comment } : {}),
        ...(evidence.length ? { evidence_refs: evidence } : {}),
        ...(revising ? { supersedes: revising.id } : {}),
      });
      sending = false;
      onrecorded();
    } catch (e) {
      sending = false;
      // A 401 has already ended the session: the sign-in page replaces everything.
      if (e instanceof ApiError && e.status === 401) return;
      refusal = e instanceof ApiError ? refusalText(e) : String(e);
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="backdrop" onclick={close}></div>
<div class="drawer sw-drawer" bind:this={panel} tabindex="-1" role="dialog" aria-modal="true" aria-labelledby={titleId} aria-busy={sending ? "true" : undefined}>
  <div class="sw-drawer__head">
    <span class="sw-drawer__grab" aria-hidden="true"></span>
    <h2 id={titleId}>{revising ? "Revise verdict" : "Record verdict"}</h2>
    <button type="button" class="sw-drawer__x" aria-label="Close" disabled={sending} onclick={close}>
      <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M3.5 3.5l9 9M12.5 3.5l-9 9" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" /></svg>
      <span class="sw-drawer__xword" aria-hidden="true">Close</span>
    </button>
  </div>
  <div class="sw-drawer__body">
    <label class="sw-field">
      <span class="sw-field__label">Subject</span>
      <select bind:value={subject} disabled={!!revising || sending}>
        <option value={`run|${runRef}`}>This run</option>
        {#each attempts as a (a)}
          <option value={`attempt|${a}`}>Attempt {a}</option>
        {/each}
      </select>
    </label>
    <label class="sw-field">
      <span class="sw-field__label">Rubric</span>
      <select bind:value={standard} disabled={!!revising || sending}>
        <option value="">Choose a rubric…</option>
        {#each all as r (r.standard)}
          <option value={r.standard} disabled={!supported(r.recognised_scale)}>{r.name} ({r.standard}){supported(r.recognised_scale) ? "" : ": scale not supported in the app"}</option>
        {/each}
      </select>
    </label>
    {#if rubrics.data && usable.length === 0}
      <p class="sw-drawer__warn">No rubric has a scale the app supports. Add one with <code>superwitness rubric-add</code>.</p>
    {/if}
    {#if scale && input}
      <ScaleInput {scale} {input} disabled={sending} onchange={(i) => (input = i)} />
    {/if}
    <label class="sw-field">
      <span class="sw-field__label">Comment (optional)</span>
      <textarea bind:value={comment} disabled={sending}></textarea>
      <span class="sw-drawer__count" class:sw-drawer__count--over={commentLen > MAX_COMMENT}>{commentLen} / {MAX_COMMENT}</span>
    </label>
    <p class="sw-drawer__ev"><span class="sw-drawer__evlabel">Evidence:</span> {evidenceSummary(evidence, kept)}</p>
    {#if tooMuch}
      <p class="refusal" role="alert">A verdict cites at most {MAX_EVIDENCE} spans and steps{kept > 0 ? `, the revised verdict's ${kept} included` : ""}; untick spans in the Trace tab or uncite steps in the Transcript tab</p>
    {/if}
    {#if refusal}
      <p class="refusal" role="alert">{refusal}</p>
    {/if}
  </div>
  <div class="actions sw-drawer__foot">
    <Button variant="secondary" onclick={close} disabled={sending}>Cancel</Button>
    <Button variant="primary" disabled={!ready} onclick={send}>{#if sending}<span class="sw-drawer__spin" aria-hidden="true"></span>{/if}{sending ? "Saving…" : "Save verdict"}</Button>
  </div>
</div>

<style>
  .backdrop { position: fixed; inset: 0; z-index: 60; background: rgb(24 22 40 / 0.42); }
  .sw-drawer, .sw-drawer :global(*) { box-sizing: border-box; }
  .sw-drawer { position: fixed; top: 0; right: 0; bottom: 0; z-index: 61; width: min(480px, 100vw); display: flex; flex-direction: column; background: var(--vk-color-surface); color: var(--vk-color-text); border-radius: var(--vk-radius-lg) 0 0 var(--vk-radius-lg); box-shadow: -20px 0 60px -20px var(--vk-color-ground); outline: none; }
  .sw-drawer__head { display: flex; align-items: center; gap: 12px; padding: 18px 20px 14px 28px; border-bottom: 2px solid var(--vk-color-line); }
  .sw-drawer__grab { display: none; }
  h2 { margin: 0; min-width: 0; font: 800 1.75rem/1.1 var(--vk-font-display); letter-spacing: -0.01em; }
  .sw-drawer__x { margin-left: auto; display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-width: 44px; height: 44px; padding: 0 10px; border: 0; border-radius: var(--vk-radius-sm); background: var(--vk-color-raised); color: var(--vk-color-text); cursor: pointer; font: 600 1rem/1 var(--vk-font-body); }
  .sw-drawer__x:disabled { opacity: 0.4; cursor: default; }
  .sw-drawer__x:focus-visible { outline: none; box-shadow: var(--vk-focus); }
  .sw-drawer__xword { display: none; }
  .sw-drawer__body { flex: 1; min-height: 0; overflow-y: auto; display: flex; flex-direction: column; gap: 16px; padding: 22px 28px; }
  .sw-drawer__foot { display: flex; gap: 10px; justify-content: flex-end; padding: 14px 28px; border-top: 2px solid var(--vk-color-line); background: var(--vk-color-surface); }

  .sw-drawer :global(.sw-field) { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .sw-drawer :global(.sw-field__label) { font: 700 0.875rem/1.3 var(--vk-font-body); }
  .sw-drawer :global(select), .sw-drawer :global(textarea) { width: 100%; min-height: 44px; padding: 10px 14px; border: 0; border-radius: var(--vk-radius-sm); background: var(--vk-color-raised); color: var(--vk-color-text); box-shadow: inset 0 0 0 2px var(--vk-color-line); font: 400 16px/1.4 var(--vk-font-body); }
  .sw-drawer :global(select:disabled) { color: var(--vk-color-muted); opacity: 1; }
  .sw-drawer :global(textarea) { min-height: 112px; resize: vertical; line-height: 1.5; }
  .sw-drawer :global(select:focus-visible), .sw-drawer :global(textarea:focus-visible) { outline: none; box-shadow: var(--vk-focus); }
  .sw-drawer__count { align-self: flex-end; font: 500 0.75rem/1 var(--vk-font-mono); color: var(--vk-color-muted); }
  .sw-drawer__count--over { color: var(--sw-danger-text); font-weight: 700; }
  .sw-drawer__ev { margin: 0; padding: 12px 14px; border-radius: var(--vk-radius-md); background: var(--vk-color-accent-wash); color: var(--vk-color-accent-text); font-weight: 600; overflow-wrap: anywhere; }
  .sw-drawer__warn { margin: 0; padding: 12px 14px; border-radius: var(--vk-radius-md); background: var(--sw-warn-wash); color: var(--sw-warn-text); font-weight: 600; box-shadow: inset 4px 0 0 var(--sw-warn); }
  .sw-drawer__warn code { font: 500 0.875rem/1.4 var(--vk-font-mono); }
  .refusal { margin: 0; padding: 12px 14px; border-radius: var(--vk-radius-md); background: var(--sw-danger-wash); color: var(--sw-danger-text); font-weight: 600; box-shadow: inset 4px 0 0 var(--sw-danger); overflow-wrap: anywhere; }
  .sw-drawer__spin { display: inline-block; width: 14px; height: 14px; margin-right: 8px; vertical-align: -2px; border-radius: 50%; border: 2px solid currentColor; border-right-color: transparent; animation: sw-spin 0.8s linear infinite; }
  @keyframes sw-spin { to { transform: rotate(360deg); } }
  @media (prefers-reduced-motion: reduce) { .sw-drawer__spin { animation: none; } }

  /* Paper: square, ruled, the title in the display face; fields are underlined lines. */
  :global(:where([data-vibe="paper"])) .sw-drawer { border-radius: 0; border-left: 1px solid var(--vk-color-text); box-shadow: none; }
  :global(:where([data-vibe="paper"])) .sw-drawer__head { border-bottom: 1px solid var(--vk-color-text); box-shadow: 0 3px 0 -2px var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) h2 { font: 500 2rem/1.1 var(--vk-font-display); letter-spacing: -0.01em; }
  :global(:where([data-vibe="paper"])) .sw-drawer__x { background: none; border-radius: 0; }
  :global(:where([data-vibe="paper"])) .sw-drawer :global(.sw-field__label) { font: 600 12px/1.3 var(--vk-font-body); letter-spacing: 0.1em; text-transform: uppercase; color: var(--vk-color-muted); }
  :global(:where([data-vibe="paper"])) .sw-drawer :global(select) { border-radius: 0; background: none; box-shadow: inset 0 -1px 0 var(--vk-color-text); font: 400 1.125rem/1.3 var(--vk-font-display); padding-left: 6px; }
  :global(:where([data-vibe="paper"])) .sw-drawer :global(textarea) { border-radius: 0; background: none; box-shadow: inset 0 0 0 1px var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .sw-drawer__ev { border-radius: 0; background: none; color: var(--vk-color-text); padding: 10px 0; border-block: 1px solid var(--vk-color-line); font: italic 400 1.0625rem/1.4 var(--vk-font-display); }
  :global(:where([data-vibe="paper"])) .sw-drawer__foot { border-top: 1px solid var(--vk-color-text); }
  :global(:where([data-vibe="paper"])) .refusal, :global(:where([data-vibe="paper"])) .sw-drawer__warn { border-radius: 0; }

  /* Studio: a hard ink edge, square fields, the evidence count set large. */
  :global(:where([data-vibe="studio"])) .sw-drawer { border-radius: 0; border-left: 3px solid var(--vk-color-text); box-shadow: none; }
  :global(:where([data-vibe="studio"])) .sw-drawer__head { border-bottom: 1px solid var(--vk-color-line); padding: 22px 24px 18px; }
  :global(:where([data-vibe="studio"])) h2 { font: 800 2rem/1.05 var(--vk-font-display); letter-spacing: -0.02em; }
  :global(:where([data-vibe="studio"])) .sw-drawer__x { border-radius: 0; background: var(--vk-color-surface); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-drawer__body { padding: 22px 24px; }
  :global(:where([data-vibe="studio"])) .sw-drawer :global(.sw-field__label) { font-size: 13px; }
  :global(:where([data-vibe="studio"])) .sw-drawer :global(select), :global(:where([data-vibe="studio"])) .sw-drawer :global(textarea) { border-radius: 0; background: var(--vk-color-bg); box-shadow: inset 0 0 0 1px var(--vk-color-text); }
  :global(:where([data-vibe="studio"])) .sw-drawer__ev { border-radius: 0; background: none; color: var(--vk-color-text); padding: 14px 0; border-block: 1px solid var(--vk-color-line); font: 400 1.0625rem/1.4 var(--vk-font-serif, var(--vk-font-display)); }
  :global(:where([data-vibe="studio"])) .sw-drawer__foot { border-top: 1px solid var(--vk-color-line); padding: 14px 24px; }
  :global(:where([data-vibe="studio"])) .refusal { border-radius: 0; background: var(--vk-color-surface); color: var(--vk-color-text); box-shadow: inset 6px 0 0 var(--vk-color-text); padding-left: 16px; }
  :global(:where([data-vibe="studio"])) .sw-drawer__warn { border-radius: 0; }

  /* Quiet: plain and flat; the close is the word. */
  :global(:where([data-vibe="quiet"])) .sw-drawer { border-radius: 0; border-left: 3px solid var(--vk-color-text); box-shadow: none; }
  :global(:where([data-vibe="quiet"])) .sw-drawer__head { border-bottom: 1px solid var(--vk-color-line); padding: 18px 24px; }
  :global(:where([data-vibe="quiet"])) h2 { font: 700 1.375rem/1.2 var(--vk-font-body); }
  :global(:where([data-vibe="quiet"])) .sw-drawer__x { background: none; padding: 0 4px; text-decoration: underline; text-underline-offset: 3px; }
  :global(:where([data-vibe="quiet"])) .sw-drawer__x svg { display: none; }
  :global(:where([data-vibe="quiet"])) .sw-drawer__xword { display: inline; }
  :global(:where([data-vibe="quiet"])) .sw-drawer__body { padding: 22px 24px; }
  :global(:where([data-vibe="quiet"])) .sw-drawer :global(.sw-field__label) { font-size: 1rem; }
  :global(:where([data-vibe="quiet"])) .sw-drawer :global(select), :global(:where([data-vibe="quiet"])) .sw-drawer :global(textarea) { border-radius: 0; background: var(--vk-color-bg); box-shadow: inset 0 0 0 2px var(--vk-color-text); }
  :global(:where([data-vibe="quiet"])) .sw-drawer__ev { border-radius: 0; background: none; color: var(--vk-color-text); padding: 14px 0; border-block: 1px solid var(--vk-color-line); font-weight: 400; }
  :global(:where([data-vibe="quiet"])) .sw-drawer__foot { border-top: 1px solid var(--vk-color-line); padding: 14px 24px; }
  :global(:where([data-vibe="quiet"])) .refusal { border-radius: 0; background: none; color: var(--vk-color-text); box-shadow: inset 6px 0 0 var(--vk-color-text); padding: 4px 0 4px 16px; font-weight: 700; }
  :global(:where([data-vibe="quiet"])) .sw-drawer__warn { border-radius: 0; background: none; color: var(--vk-color-text); box-shadow: none; border: 2px dashed var(--vk-color-text); }

  /* A phone: a full-width sheet from just under the top, Cancel and Save side by side and sticky. */
  @media (max-width: 759px) {
    .sw-drawer { top: 56px; width: 100vw; border-radius: var(--vk-radius-lg) var(--vk-radius-lg) 0 0; border-left: 0; }
    .sw-drawer__grab { display: block; position: absolute; top: 8px; left: 50%; width: 36px; height: 4px; margin-left: -18px; border-radius: 2px; background: var(--vk-color-line); }
    :global(:where([data-vibe="daylight"])) .sw-drawer__head { position: relative; padding-top: 22px; }
    :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .sw-drawer { top: 0; border-radius: 0; }
    :global(:where([data-vibe="paper"], [data-vibe="studio"], [data-vibe="quiet"])) .sw-drawer__grab { display: none; }
    .sw-drawer__head { padding: 14px 16px 12px 20px; }
    h2 { font-size: 1.5rem; }
    .sw-drawer__body { padding: 16px 20px; }
    .sw-drawer__foot { position: sticky; bottom: 0; padding: 12px 16px calc(12px + env(safe-area-inset-bottom)); }
    .sw-drawer__foot > :global(*) { flex: 1 1 0; }
    .sw-drawer__foot :global(.vk-button) { width: 100%; }
  }
</style>
