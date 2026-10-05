# Security

## Reporting a vulnerability

Please report security issues **privately**, through GitHub:

1. Open the repository's **Security** tab.
2. Choose **Report a vulnerability**.

That opens a private advisory that only the maintainers can see. Please don't
open a public issue or pull request for a suspected vulnerability.

A useful report includes the version (`superwitness --version`), what you did,
what happened, and what you expected. If you have a proof of concept, include
it in the advisory.

You'll get an acknowledgement within a few days. Fixes are released as a new
patch version with a published advisory, and you'll be credited unless you'd
rather not be.

## Supported versions

superwitness is pre-1.0. Security fixes go into the latest release only, so
please upgrade to it before reporting.

## Scope

The most sensitive parts of superwitness are these:

- **Verifying hub-issued tokens.** That covers audience, signature, expiry and
  key refresh.
- **The verdict store's append-only guarantees.** That covers the database
  triggers and the separate migration and runtime roles.
- **Keeping prompt, message and tool content out of telemetry.**

Reports about any of these are especially welcome.
