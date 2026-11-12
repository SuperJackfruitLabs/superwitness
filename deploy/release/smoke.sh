#!/bin/sh
# The release's production smoke (`smoke` in .github/release-config.json; the workflow retries
# it): the staging smoke against the public URL, signing in at production Accounts.
set -eu
EXPECT_ISSUER=https://accounts.superjackfruit.com exec sh deploy/staging/smoke.sh https://app.superwitness.dev
