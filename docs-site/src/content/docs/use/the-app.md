---
title: The app
description: Signing in with AgentPod, browsing runs, reading a run and recording verdicts in the browser.
---

superwitness serves a web app from the same binary and address as its API. People on an
allowlist sign in with their AgentPod account and can browse the runs sources have reported,
read any run, record verdicts against rubrics and revise them. Agents and scripts keep using
the [HTTP API](/build/api/) and the [MCP tools](/build/mcp/) with bearer tokens.

## Turning it on

The app is off until someone is allowed to sign in. Four settings turn it on
([Configuration](/use/configuration/)):

| Setting | Meaning |
|---|---|
| `SW_ALLOWED_PRINCIPALS` | comma-separated AgentPod principal ids (`prn_…`) who may sign in. Empty, nobody may, and the app is off. |
| `SW_APP_CLIENT_ID` | the AgentPod OAuth client browsers sign in through. |
| `SW_SESSION_SECRET_FILE` | a file holding at least 32 random bytes, readable by superwitness alone. It keys the short-lived cookie that carries a sign-in to AgentPod and back. |
| `SW_TRUSTED_PROXIES` | addresses or CIDR prefixes allowed to say who a client is with `CF-Connecting-IP`. Unset, it is this host's own addresses. |

`SW_PUBLIC_URL` must be `https` when sign-in is on: the app's cookies are `Secure`, and a browser
drops them over plain http. (Fake mode, which listens on loopback only, is the exception.) Make
the secret with, for example:

```sh
head -c 48 /dev/urandom | base64 | sudo tee /etc/superwitness/session-secret >/dev/null
sudo chmod 0600 /etc/superwitness/session-secret && sudo chown superwitness /etc/superwitness/session-secret
```

At AgentPod, the OAuth client named by `SW_APP_CLIENT_ID` must list `{SW_PUBLIC_URL}/auth/callback`
as a redirect URI and `SW_PUBLIC_URL` as an audience. superwitness also needs its own service
credential to hold `evidence:read`, as it already does to read attempts: it uses it to look up
who signed in.

## Signing in

**Sign in with AgentPod** sends the browser to AgentPod, which signs the person in if they are
not already and sends them back to `/auth/callback` with a one-time code. superwitness trades
the code for a token server to server, with PKCE, checks the token, and looks up the AgentPod
principal behind it. It then checks that the principal is a person, is not suspended, and is on
`SW_ALLOWED_PRINCIPALS`. Anyone else, including other valid AgentPod accounts, agents and
services, sees **Not authorised**. Each refusal is logged as `auth.signin_refused` with a reason,
for example `not_allowlisted`, `login_missing` (the sign-in cookie was absent) or `no_code` (the
callback carried no code); the log never holds a token or a cookie.

The sign-in itself rides on a signed `__Host-sw_login` cookie that lives ten minutes and is used
once. A signed-in browser then holds a `__Host-sw_session` cookie and a superwitness session,
never an AgentPod token. Over plain http (fake mode only) the cookies are `sw_login` and
`sw_session`, without the `__Host-` prefix.

- the session lasts at most 12 hours, and ends after 2 hours unused;
- every request checks the allowlist again, so taking someone off it (and restarting
  superwitness) ends their access at their next click;
- **Sign out** (`POST /auth/logout`) ends the session at once and clears the cookie. If the
  database cannot be reached the answer is 503 `store_unavailable`, with the cookie cleared
  all the same, and the session expires on its own;
- sessions do not need AgentPod: if AgentPod is down, open sessions keep working and only new
  sign-ins wait.

Changes made with a session must come from the app's own page (their `Origin` must equal
`SW_PUBLIC_URL`); anything else is refused with 403 `origin_mismatch`. Requests with a bearer token
are not affected, and a request that carries a bearer token is judged by the token alone. Run
reports (`POST /v1/runs`) and `/mcp` never accept a session.

## Pages

- **Runs**: every run sources have reported, newest first, grouped under Today, Yesterday and
  earlier dates in your own time zone. The sidebar has All, Needs verdict (finished runs no
  verdict names yet), Failed and Waiting, and a view per board. Each card shows the title, the
  status, the board, who ran it, how long it took or when it started, and its latest verdict.
  The filters are in the address, so a view can be bookmarked. With no runs yet, the page links
  to the [Run registry API](/build/run-registry/).
- **A run**: its source and reference, title, status and **Record verdict**; a strip with the
  agent, attempts, errors, duration and cost; and tabs for the trace (a span waterfall, 500 spans
  at a time), logs (with a level filter), errors, verdicts (with revisions shown as history: a
  superseded verdict is struck through and links to the one that replaced it) and attempts (each
  with its configuration fingerprint). A source that did not answer shows as unavailable, and a
  value no source supplied reads `unknown`, never 0. A run missing from the list still opens by
  its address, `/runs/superpipeline/{board}/{run}`.
- **Rubrics**: every rubric version, and each one's scale and text. Rubrics are added with
  `superwitness rubric-add`.

The sidebar's dot is green when every source answers `/health`, amber otherwise; hover for which.
The app follows the system's light or dark setting; the toggle in the sidebar overrides it for
this browser. Below 760 px wide the sidebar folds into a menu.

## Recording a verdict

**Record verdict** opens a drawer. Choose the subject (the run, or one of its attempts) and a
rubric, then give the value the rubric's scale asks for ([rubric scales](/use/verdicts/#rubric-scales)):
a decision, a score from 0 to 1, a label, or text. Add a comment of up to 10,000 characters if
you like. Spans ticked in the Trace tab, up to 100, go with it as evidence. A rubric whose scale
the app cannot offer an input for is listed but cannot be chosen.

The drawer makes its idempotency key when it opens, so a double click or a retry records one
verdict. **Revise** appears on your own latest verdicts: it opens the drawer with the same subject
and rubric and records a verdict that supersedes the old one. The app explains refusals in plain
words, for example "You've already revised this verdict" when someone revised it first.

## Rate limits

superwitness limits requests in its own process, by client address or by principal:

| What | Limit |
|---|---|
| `/auth/*` | 10 a minute per client address, bursts of 20 |
| changes made with a session | 30 a minute per person, bursts of 10 |
| `POST /v1/runs` | 600 a minute per reporter, bursts of 100 |

Over a limit the answer is 429 `rate_limited` with `Retry-After`. The client address is the
connection's, unless the connection comes from `SW_TRUSTED_PROXIES`, in which case it is the
`CF-Connecting-IP` header.

## Putting it on the internet

superwitness serves the app, the API and `/mcp` on one address. To publish the app through a
tunnel or a reverse proxy, point the proxy at superwitness from an address in
`SW_TRUSTED_PROXIES` and have it set `CF-Connecting-IP`. A request that arrives that way, through
the public edge, is answered 404 on `/mcp` before any authentication, so the MCP tools stay on
your private network. Callers on the private network, which carry no `CF-Connecting-IP`, are
unaffected. Set `SW_PUBLIC_URL` to the public https address, and
add that address as an audience to every AgentPod client whose tokens call superwitness, before
you change it, so that tokens carry both the old and the new audience during the switch.

## When something is down

| What | What you see |
|---|---|
| AgentPod, during sign-in | "AgentPod sign-in is unavailable"; open sessions keep working |
| a source | that run's tab says it is unavailable; the sidebar dot turns amber |
| superwitness's database | "Can't reach the database" |
| a link to a run no source knows | "Run not found" |
