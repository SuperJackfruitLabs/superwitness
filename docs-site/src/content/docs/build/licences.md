---
title: Licences
description: The licences of superwitness and of everything it links or runs beside.
---

superwitness is released under the [MIT licence](https://github.com/SuperJackfruitLabs/superwitness/blob/main/LICENSE).

## What ships

A release tarball holds:

- the `superwitness` binary, with the app built into it;
- `LICENSE` and `NOTICE`;
- `licenses/fonts/`, the fonts' licences;
- `deploy/superwitness.service` and `deploy/env.example`.

The binary links Go modules and embeds a web app built with React, with three bundled fonts. Each keeps its own licence,
and the [`NOTICE`](https://github.com/SuperJackfruitLabs/superwitness/blob/main/NOTICE) file in
every release credits them. In summary:

| Component | Licence |
|---|---|
| chi, pgx and its jackc helpers, goose and mfridman/interpolate, golang-jwt, google/jsonschema-go, segmentio/encoding, cespare/xxhash, felixge/httpsnoop, go.uber.org/multierr, cenkalti/backoff | MIT |
| the MCP Go SDK | MIT for existing code, Apache-2.0 for new contributions |
| segmentio/asm | MIT-0 (MIT No Attribution) |
| sethvargo/go-retry, go-logr/logr and stdr, the OpenTelemetry Go API, SDK, exporters, otelhttp instrumentation and OTLP protobufs, gRPC and genproto | Apache-2.0 |
| portions of the OpenTelemetry Go modules and otelhttp, from the Go Authors | BSD-3-Clause |
| yosida95/uritemplate, google/uuid, grpc-gateway, Go protobuf, and golang.org/x sync, net, oauth2, sys, text and time | BSD-3-Clause |
| react, react-dom and scheduler, in the embedded app | MIT |
| Fraunces, IBM Plex Sans and IBM Plex Mono, bundled in the app (from the `@fontsource` packages, also OFL-1.1) | SIL Open Font License 1.1 |

The list covers what the binary links, not test-only modules. Build and test tools are not bundled: the
app's development dependencies, including caniuse-lite (CC-BY-4.0, used by the browser-list
tooling), @playwright/test (Apache-2.0) and jsdom (MIT), stay out of the release. The same goes for the npm packages that build
superwitness.dev and this documentation site: they are not part of any release.

## The allowlist

superwitness's CI checks the licence of every Go module it builds with, using `go-licenses`,
against an allowlist of permissive licences:

- MIT
- Apache-2.0
- BSD-2-Clause
- BSD-3-Clause
- ISC

`go-licenses` does not classify `segmentio/asm`, which the MCP SDK pulls in. Its licence, MIT-0,
was reviewed by hand: it is permissive, and the allowlist is unchanged.

No AGPL code is linked or shipped, so Grafana, Loki, Tempo and Mimir are not used, and neither
is anything under a source-available licence such as ELv2, BSL, FSL or SSPL.

## What runs beside it

The telemetry engines are not part of superwitness. They are separate upstream programs that
you install and run as released, and superwitness only calls their published APIs over HTTP.
This release queries VictoriaTraces and VictoriaLogs, fed by the OpenTelemetry Collector. Each
is under its own licence:

| Project | Licence |
|---|---|
| [VictoriaTraces](https://github.com/VictoriaMetrics/VictoriaTraces) | Apache-2.0 |
| [VictoriaLogs](https://github.com/VictoriaMetrics/VictoriaLogs) | Apache-2.0 |
| [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) | Apache-2.0 |

superwitness is designed to sit beside [VictoriaMetrics](https://github.com/VictoriaMetrics/VictoriaMetrics)
and [Perses](https://github.com/perses/perses) (both Apache-2.0) the same way, but this release
does not use them.
