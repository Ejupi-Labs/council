# Performance measurements

Run `npm run benchmark` from a development checkout. It builds the runtime, creates its own temporary local SQLite database, asserts equivalent summary fields and cancellation outcomes, reports JSON, and removes its fixture. It never opens an existing project database or calls a provider. There are no timing pass/fail thresholds.

The fixture contains 20 runs averaging about 1.29 MB each, 73 events, one active foreign run and 20 simultaneous 500 ms status waiters. Two warmup iterations precede ten measured listing and cancellation operations. The baseline reads full snapshots/events and then derives the same compact answer; the new path reads indexed persisted metadata. This compares algorithms in the same runtime, rather than promising a speedup for every project.

One measurement on Windows ARM64 with Node v24.18.1:

| Operation | Full snapshot/event path | Indexed metadata path |
| --- | ---: | ---: |
| Ten listings | 671.7 ms | 3.6 ms |
| Ten cancellation checks | 4.4 ms | 0.15 ms |
| Foreign summary waiters | Full transcripts would be about 1.41 MB per read | 17.3 KB per read; zero full-record reads |

The waiter workload made 120 summary reads in about 505 ms and decoded about 2.08 MB in total. Replacing each with the equivalent full record would decode about 169.5 MB, so this fixture reduces decoded polling payload by approximately 98.8%. Process CPU measurements have platform-dependent resolution; a reported zero does not mean an operation consumes no CPU.

`tests/storage-load.test.mjs` uses another fixture with large selected outputs on every run. Its 20 raw records total 26,229,051 bytes and its summaries total 332,381 bytes: 78.9 times less materialized data. It also exercises indexed cancellation and call-slot acquisition without decoding live run documents.

Persisted summaries and covering indexes trade additional database space and write work for bounded reads. Full results still require an explicit transcript read. Integrity checks, backup, initial schema migration and actual provider latency are separate workloads; these measurements do not establish their throughput or a hosted-service SLA. Hardware, filesystem, antivirus and concurrent load affect elapsed time. Run the script on the deployment machine when choosing operational capacity.

## Version 1.2 stress fixtures

The following measurements use disposable local fixtures on the same Windows ARM64 development machine. They isolate the indicated query or algorithm; they do not measure provider latency or end-to-end workflow throughput.

| Fixture | Earlier path | Version 1.2 path |
| --- | ---: | ---: |
| 100 unread-inbox lookups after 100,000 acknowledged messages | 1,841 ms through the earlier API | About 1 ms for the equivalent indexed SQL probe |
| 100 health reads after 100,000 expired leases | 3,383 ms | 10–14 ms with the covering expiry index |
| Warm summary of 2,000 retained artifact entries | 753 ms | 0.17 ms with bounded inspection |
| Forty 1.1 MiB escaped quoted frames, scanner CPU only | 265 ms | 47 ms |

The inbox comparison uses the earlier public API and an equivalent indexed SQL query. The separate permanent full-API regression uses 50,000 acknowledged messages and measured 2.06–2.41 ms for 100 polls. Summary generation inspects at most 64 complete artifact entries, preserves their full identities, and marks omitted entries. Explicit full-record reads still return the original evidence. The JSON scanner comparison retains equivalent escape and structural validation; differential tests cover valid and malformed frames.

A final version 1.2 build measurement during concurrent endurance tests recorded ten full-snapshot listings at 574.1 ms and ten metadata listings at 3.6 ms. It confirmed equal shared fields, equivalent cancellation results and zero full-document reads for twenty foreign waiters. Those waiters decoded 2.08 MB of summaries over 120 polls; one equivalent full record was 1.41 MB. An earlier loaded run measured 1,502.7/24.9 ms for the same listing algorithms, illustrating the effect of machine load.

A sustained four-writer run exposed online-backup starvation after the database reached about 19 MB. In a controlled copy of that fixture, a foreign commit after every 128-page backup step left the unpinned copy at 4,518 of 4,646 pages remaining for 100 steps. Pinning a dedicated read snapshot completed in 36 callbacks and 333.82 ms while all 36 foreign writes continued; the backup contained the prewrite counter and passed integrity checking. The copy retains 128-page steps, cancellation checks and the 60-second budget. This is a fixture result, with WAL retention lasting until the snapshot closes.

Fair queue admission also changes latency under contention. In a controlled three-slot fixture, strict head-only admission delayed eligible B/C callers by 274/526 ms while free capacity waited for older A to poll. Reserving capacity for the older prefix admitted B/C in 13/3 ms, while A retained its reservation and acquired at 271 ms. This removes avoidable polling serialization; it does not promise a maximum wait or a throughput multiplier. Queue waits remain cancellable and have no independent deadline.

The artifact harness stops at its requested duration or its explicit/default 1,000-cycle ceiling, whichever comes first. Its summary records the requested duration, observed workload time, cycle limit and stopping reason; reaching the cycle limit does not qualify the full requested duration. Setup and cleanup timings are separate. The 128 MiB fixture budget remains enforced independently.

The storage endurance harness rotates its disposable fixture after five minutes, or earlier when another backup would lack headroom under the existing 256 MiB limit. Each rotation first stops every worker, requires its final report, checks that terminal-cycle counts match retained acknowledged mail, verifies no active runs/slots/waiters/instances/leases remain, and checks database integrity. Only then does it remove that fixture and start the next one. Production message history is never pruned by this harness. A cleanup or verification failure preserves the fixture and fails the run. The aggregate summary distinguishes total operations across fixtures from the final fixture's database counts; setup and draining can extend elapsed time beyond the requested duration. `--fixture-ms` can shorten fixture lifetime for offline harness checks, within 1,000–300,000 ms.

For sustained coordination and artifact evidence, see the [release verification record](verification.md). The service workload bounds retained history and rotates its fixture every fifteen minutes, so its memory observations describe that retention policy rather than unlimited transcript growth. `npm run soak`, `npm run soak:storage` and `npm run soak:artifacts` create their own temporary workspaces and make no provider calls.
