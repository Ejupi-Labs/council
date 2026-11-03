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
