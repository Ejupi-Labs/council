# Workspace foundation review

Date: 2026-11-05. Branch: refactor. Outcome: accepted for the foundation scope on Windows ARM64.
This is not acceptance of managed execution or completion of the product roadmap.

## Independence and scope

| Review | Reviewer | Outcome |
| --- | --- | --- |
| Product baseline and feature design | Separate requirements/design reviewers | 15/15 WS requirements and 7/7 success criteria mapped; no blocking inconsistency |
| domain/app/store | UI implementer, who did not author those packages | No actionable finding; fresh scoped Go tests and vet passed |
| localstate/project/discovery and UI | Core implementer, who did not author those packages | Native tests passed; two UI bugs reproduced in real Chromium, corrected and independently rechecked |
| CLI/HTTP | Native-package implementer, who did not author CLI/HTTP | Eight findings corrected; uncached scoped tests and vet passed |
| Packaging/build/browser journey | Root integrator, reviewing the packaging/UI authors' work | Explicit archive allowlist, licenses, checksums, extracted binary and real UI verified |

The ignored development reports under .council/refactor/reviews retain detailed working notes.
This committed record retains the findings, closure and verification boundaries needed by a reader.
The reviewed implementation is committed together with this record; design precedes it at3e9cb4f,
with preservation-plan correction219fa2f. No application implementation preceded that design gate.

## Corrected findings

| ID | Problem | Correction and observed evidence |
| --- | --- | --- |
| UI-01, P1 | A real committed draft followed by a lost acknowledgement and401 could lose its retry identity and create a duplicate after reconnect. | Pending draft/open/policy intents latch unresolved state across access/preflight errors and reload recovery. Browser regression commits once, loses acknowledgement, receives401, retries while unverified, reconnects, then sends the same ID/body; exactly one draft remains and newer typing survives. Independently rechecked. |
| UI-02, P2 | Explicit delayed policy reload could overwrite a newer dropdown choice. | Every policy read checks its captured edit generation. Browser regression delays response, changes the selection and confirms it remains; independently rechecked. |
| CLIHTTP-01 | Early parser errors could ignore requested JSON output. | One error envelope/no stderr for flags before or after --json; regression passes. |
| CLIHTTP-02 | Empty IDs/filters became fresh IDs or omitted filters; version accepted unrelated flags. | Reject present-but-empty values and unsupported version options; regression passes. |
| CLIHTTP-03 | Oversized UTF-8 task file cut inside a codepoint could report the wrong error. | Size check precedes decoding; regression reports input_too_large. |
| CLIHTTP-04 | Owner/storage failures could replace a supplied mutation identity. | All such branches preserve the validated request identity. Independently source-inspected; no injected home-directory failure test claimed. |
| CLIHTTP-05 | Unexpected mutation panic could lose an uncertain commit's retry identity. | HTTP retains validated ID and returns write_uncertain; test preserves ID and omits panic detail. Transient CLI recovery also preserves it. |
| CLIHTTP-06 | Forwarded mutation could be accepted without a valid receipt. | Require matching request ID, operation and canonical timestamp; disallow receipt on errors/reads. Valid/malformed response regressions pass. |
| CLIHTTP-07 | Owner verification ignored scope/execution declarations. | Compare exact foundation scope and capability declarations as well as instance/API/app versions; independently source-inspected. |
| CLIHTTP-08 | HTTP accepted undocumented media-type parameters. | Only UTF-8 charset allowed; invalid parameter rejected before core dispatch. |

All reported blocking findings are closed. Reviews did not authorize new features or live calls.

## Cross-platform CI correction

Run42110748901 exposed canonical-spelling assumptions in the private state owner and project
fixture assertions on macOS and Windows hosted runners. Ordinary ancestor aliases now resolve to
one physical owner root after leaf/same-file validation. A redirected v2 leaf or private state
file is still rejected. Windows uses metadata-only GetFinalPathNameByHandle for junction ancestors;
Unix uses filesystem canonical resolution. The same helper fixes an independently reproduced
project-through-junction failure; its regression failed before the fix and passes after it.
Physical UTF-16 conversion rejects malformed scalar/NUL input and preserves literal U+FFFD. Tests exercise alias/canonical lock exclusion,
descriptor parity/cleanup and untouched redirected-file sentinels. Root independently inspected
the production diff and platform fixture helpers; native author reran uncached Windows ARM64
tests/vet and six-target compilation. Runtime CI of the correction remains separately reported.

The root also aligned UI text admission with Unicode White_Space, preserving U+FEFF-only valid
content consistently with the server. The native author independently reviewed that narrow change;
the actual browser regression saves it and verifies exact CLI retrieval.

## Limits

Executed runtime qualification is Windows ARM64, Go1.27.3, SQLite3.54.1, Chromium151.0.7922.34.
Native packages cross-compile for six OS/architecture targets. Unix runtime, Windows AMD64 runtime
and Linux race jobs remain contingent on actual CI results; no cross-compilation is counted as a
runtime pass. No vendor executable, credential access, live provider task or saving measurement
was involved. Future adapters/execution require their own specification and review.
