# 008 Design Review

Independent pre-implementation review identified and corrected these design issues:

- A known older owner must retain its separately verified maintenance Stop without allowing new execution or replacing its lock; owner capability checks are per operation and tool discovery is available before attachment.
- Existing owner receipts intentionally normalize certain omitted/default inputs. Host retries preserve those documented semantic equivalences while effect-changing input or captured authority conflicts; an adapter-local raw-byte cache cannot redefine cross-interface behavior.
- The SDK alone does not enforce the initialized notification before dispatch. An ordered per-session validating gate and raw stdio tests are required.
- The original catalog duplicated agent list/refresh; one fresh observational list tool now represents the existing route. A mechanical audit must reject other missing, extra or duplicate mappings.
- Owner DTO references alone are not complete advertised JSON schemas. A dedicated versioned schema generation and independent review gate precedes all tool dispatch implementation.

At that design-review point, implementation and qualification were not yet established. Later independent source reviews and the exact-source acceptance evidence are recorded below; live provider qualification remains separate.

## Historical implementation review

The initial SDK integration exposed two interoperability defects. The SDK may
dispatch an ordinary request asynchronously after a later initialized
notification; the raw transport now gates frames in wire order and waits for the
SDK's successful initialized acknowledgement. The default SDK client also
probes `server/discover` before falling back to the qualified 2025 lifecycle;
that probe is allowed to reach the SDK without enabling application operations.
Focused real-pipe tests cover both paths. Transport shutdown requires streams
whose `Close` interrupts I/O and handlers that honor their call context; the
eventual entrypoint and dispatcher must preserve those conditions.

Independent schema review found missing decimal, wait and byte-page bounds and
cross-field constraints. These were corrected in the candidate schemas. Some
owner constraints, including encoded byte limits, graph relations and node-ID
uniqueness, cannot be expressed exactly by JSON Schema. A separate pre-dispatch
body-validation layer uses the owner's strict decoder and pure domain validators;
all fixed-route mutations have an explicit entry. The owner-client route guard
independently validates all path identities and allowed query names before owner
attachment. Both checks are mandatory; neither grants permission or replaces
stateful owner validation. Independent review approved the 91-schema inventory
after the focused parity, bounds, routing and authority tests; each approved
schema is now pinned to its compact JSON token digest. A changed or missing
schema fails closed and requires renewed review. The review was targeted rather
than exhaustive, and real MCP dispatch and authenticated owner parity are
still required before feature acceptance.

Independent owner-client review also found that decoding `data` into an
untyped interface rounded JSON integers above 2^53. The owner client now keeps
the validated data as raw JSON while separately validating status, receipt and
error. A local regression covers nested large integers, decimal precision,
null values and receipt replay. The eventual MCP result serializer must retain
that raw JSON; decoding it back through a floating-point interface would
reintroduce the defect.

T010 review identified an additional raw-protocol issue: the SDK discards
unknown members inside decoded tool parameters. The transport now checks the
2025 `tools/call` and `tools/list` parameter shapes before the SDK can discard
them, while retaining extensible `_meta`. Raw tests show invalid parameters
close the connection before the owner is called. Initial real-subprocess tests
also observed two SDK clients reuse the same owner and survive one client's EOF;
these focused results do not replace the full cross-interface and retry gates.
The local SDK-client subprocess fixture additionally opened a Unicode/space-path
project through CLI, ran separate fake tasks through MCP and CLI, and inspected
the same identities, original instructions and settled outcomes through the
opposite interface. It used a disposable home and Git project, not a live
provider. An authenticated older-owner fixture allowed only the recognized
Stop; incompatible and malformed locked-owner fixtures retained their lock.
The Playwright browser fixture exercised a fake task created from MCP and a
second created in the UI, then read each through the other interface with the
same task/attempt IDs and original Unicode instructions. A separate authenticated
HTTP fixture committed a policy mutation in the real application store, dropped
the acknowledgement, and observed `write_uncertain`, the original receipt on
retry and a conflict for changed intent. It is not a live-provider test.
An additional real stdio fixture sent and cancelled a pending wait and disconnected
the host while a slow fake worker remained running under the same owner. A
subsequent explicit Stop retained a cancelled outcome, verified cleanup and one
matching stop event. An earlier 34-second test used a fixture close that waited
for the outstanding request; it was corrected to send EOF explicitly and use
an eight-second host-exit watchdog before accepting the result.
An evidence fixture compared MCP and CLI byte pages for a real UTF-8 fake-worker
artifact, then used a controlled local check to produce 450,000 binary bytes.
It captured a growing prefix, retained its `through` boundary after closure,
reconstructed pages up to 65,536 bytes and verified the complete SHA-256. The
first binary fixture used an incompatible fake scenario and timed out; after
selecting the supported controlled scenario the test passed.
An SDK/owner/CLI routing fixture exercised policy, native context and profile
updates, a declared account and link, inherited capacity reporting, a fake task
and budget grant. Usage and charge fields remained unknown where the fixture did
not report measurements; it did not qualify a provider. Another isolated owner
fixture verified replay and changed-intent conflicts for graph start/stop,
graph pause and metadata-history capture/preview; checkpoint resume and history
publication are not covered by that particular test. The setup fixture launched
the executable directly from a path with spaces and Unicode and listed the
catalog without starting an owner.
An isolated fake-workflow journey retained a failed check and review finding,
then used a CLI budget grant and MCP advance to reach an approved review. A
dependent graph subsequently bound the receiver input to the exact predecessor
attempt and explicit CLI response acceptance before both nodes completed. The
first fixture incorrectly decoded the acceptance wrapper; the corrected test
checks the receipt and complete response binding.
The extended workflow test compared all candidate records and the `review_only`
delivery between MCP and CLI, without applying the candidate. It also held two
fake sibling workers on distinct contexts sharing one declared account: a
controlled barrier showed one reservation, a waiting sibling, and nonoverlapping
execution after release. An initial fixture used graph `author_only`, which the
owner correctly rejects; the corrected fixture used supported graph workflow.
The recovery/history fixture also paused and resumed a graph member after the
fake root input was ready, published an incomplete metadata export, then
reconciled a separate blocked publication without changing a pre-existing file.
Two earlier fixture attempts tried to resume before the root input was ready and
correctly returned `graph_input_stale`; the final test waits for the actual
ready/current binding before retrying.
The extended fixture injected two distinct retained incidents: MCP reconciled
the history-linked incident while CLI/store observation confirmed that the
independent incident and its dispatch fence remained in place. The incidents
were controlled fixture injections, not claims of a naturally occurring fault.
The MCP cancellation-edge fixture also checked a real wait timeout, pause while
draining, partial failure with retained output, and cleanup uncertainty with a
dispatch fence. Its `late_success` fake was stopped before emitting success: it
demonstrates Stop precedence, not receipt of a success after Stop.
Separately, a controlled test accepted Stop through MCP while a preparation
barrier guaranteed no process had been created, injected a `succeeded`
settlement through the real store, and confirmed via MCP that the retained
outcome and Stop receipt remained cancelled. This tests settlement precedence;
it does not claim that a native process emitted a late success.

The subsequent independent T010/US4 audit traced graph and checkpoint authority
from raw arguments through schema and owner-body/query validation, exact path
binding and authenticated forwarding. It found no additional concrete defect;
state-dependent proof and cursor checks remain with the owner. This targeted
review does not replace full-suite, hosted or live-provider qualification.
The final independent source review found no further concrete blocker in the
transport, dispatch and binding, shared owner compatibility, entrypoint,
catalog, guide or dependency notices. It retained the qualifications above:
many real-owner catalog rows exercise nonexistent-resource errors, the
recorder is not evidence of engine effects, the owner restart was orderly, and
specific editor/provider hosts and live providers were not qualified. Full
local commands, exact-source hosted jobs and the acceptance record remained open at that point; see the final CI record below.

During the first complete local `npm test`, an independent cross-platform audit
found three test-only reliability issues after the `internal/mcphost` package
had already run: a marker could be read before its writer closed, a subprocess
`Wait` could close stdout before the scanner drained it, and a one-second fake
could settle before Stop while a separate shutdown error was suppressed. The
fixtures were corrected while the suite was running later packages. The MCP
package results from that run therefore precede those test-only changes; rerun
the affected tests, record this mixed test-source provenance and require hosted
qualification on the exact final source. No product source was changed by these
three fixture corrections.
An additional raw-protocol test confirmed that a `tools/call` notification
without a JSON-RPC request ID does not reach the owner or produce a reply; the
same mutation with an ID reached the fixture once. This check is provided by the
pinned SDK and is now regression-tested at Council's transport boundary.
An additional CLI→MCP retry fixture compared receipts for checkpoint resume,
metadata-history export and reconciliation, and observed conflicts when node,
graph, task, scope, preview or revision authority changed. It used the actual
engine/store and controlled fake workers in a disposable environment.

The authenticated catalog recorder exercised all 91 tool mappings and checked
the exact HTTP operation, path-identity order, query, body, status and receipt;
its executor is a fixture and does not establish engine semantics. A separate
real-owner catalog test subsequently called every tool in a disposable home.
For 90 rows it compared MCP status, error code and mutation receipt with the
same fixed request through the authenticated owner client; the final explicit
owner Stop was checked once, last, because it shuts the owner down. Several
rows deliberately use nonexistent resource identities and therefore prove the
actual owner's error path rather than successful state transitions. The first
version of this test expected Stop status 200; the real owner correctly returned
202, and the corrected test passed. The focused journeys above provide successful
state-transition evidence for their named families.
Further retry fixtures closed the first MCP subprocess after a lost HTTP
acknowledgement or an MCP cancellation after commit. A new host sent concurrent
retries with distinct JSON-RPC IDs and retained one task/attempt and the same
receipt, including CLI replay; changing the intent conflicted. These tests used
an actual engine/store with a controlled non-launching process preparer. They do
not claim to have restarted the owner process.
An additional test did restart the actual owner process in an orderly manner:
a local authenticated proxy dropped a post-commit acknowledgement, the first
owner was explicitly stopped, and a new owner on the same disposable home
returned the original receipt to MCP and CLI without another attempt. This does
not establish crash or dirty-epoch recovery.

## Final local recheck before hosted qualification

The complete `npm test` passed on Windows ARM64 on the final product source, including 97 browser tests and three script/package tests. The MCP package in that run preceded the three test-only fixture corrections documented above. The corrected affected tests were rerun together with `go test -p 1 ./internal/mcphost -run '^(TestMCPWorkflowAndDependentGraphAcrossCLI|TestMCPSubprocessRejectsStaleOwnerIdentity|TestMCPCancellationAndPartialOutcomeEdges)$' -count=1 -v` and passed (54.976 seconds). `npm run build`, `npm run check`, and the notice inventory check passed; exact committed-source hosted qualification was still open at that point; see the final CI record below.

## Post-push protocol and Unix fixture review

Independent review found valid tool calls with invalid parameters closed the session instead of returning a correlated error. It also identified SDK float64 conversion of incoming numeric JSON-RPC IDs. The corrected transport returns sanitized `-32602` without owner dispatch, preserves string and integral numeric identity through disjoint SDK-internal tags, maps cancellation targets and maintains original owner-bound bytes. Independent review covered concurrency, directionality, the initialized gate and notifications. Focused real-SDK tests passed; full corrected-source qualification was pending at that point; see the final CI record below. To bound resource use, nonzero numeric IDs whose normalized exponent text, including a negative sign, exceeds 20 characters are rejected as invalid frames; string IDs are available instead.

Preliminary hosted run 42448475324 for 6f38e14 failed on Unix: the MCP test binary lacked the native process guardian entrypoint and a child Go module cache was created inside a temporary HOME with read-only files; macOS ARM64 also exposed a reconnect-fixture close race. The fixtures now register the guardian, reuse the external Go module cache and drain SDK stdout before process wait. A subsequent local run exposed an expected duplicate stdin close; the trace now closes its underlying writer once while preserving real errors. Independent review accepted these fixture-only corrections, and the four affected focused cases passed on Windows ARM64 in 25.987 seconds. Corrected-source full and hosted runs were mandatory; the final record is below.

## Follow-up Windows graph fixture

Candidate run [42450778981](https://github.com/Ejupi-Labs/council/actions/runs/42450778981) for `d8374f9f60df0fda265d117eac6c8dd7ec638296` failed Windows ARM64 in `TestGraphNodeStopCancelsLiveBranchAndBlocksWaitingFollowerWhileIndependentSiblingFinishes` before either worker reached its ready observation: both attempts were admitted but still preparing at the original deadline. The log does not prove whether preparation was merely slow or stuck. The test now allows 30 seconds for its first actual ready worker, then separately requires both ready within six seconds; the fake worker release deadline, exact Stop, independent sibling, blocked follower and cleanup assertions remain unchanged. Independent review accepted this as a bounded fixture change and noted that a retained ready marker plus a running database projection is not independently conclusive OS-process liveness. The changed test and complete final candidate require fresh qualification; repeat preparation failure at the longer bound requires investigation, not another unsupported timeout increase.

The corrected product completed `npm test` successfully on Windows ARM64 (26 engine batches, 97 browser tests and three script/package tests). Because the graph readiness test changed after its batch ran, the final test was rerun separately and passed in 9.098 seconds. `npm run check` and the notice check passed; the full seven-job run on exact candidate `bb6773278fad39c02d451fe2dcfc03e927d5e46a` remains the final gate.

## Shared-account CI observation requiring resolution

Candidate run 42452268832 for bb67732 failed Linux ARM64 in `TestMCPWorkflowAndDependentGraphAcrossCLI`: the shared-account check found that the observed claim or budget no longer matched the first attempt before explicit fixture release. The original assertion did not log which condition changed and therefore proves neither overlapping account use nor a simple timeout. Independent analysis found the same test passed Linux ARM64 and AMD64 and macOS ARM64 on the preceding candidate; the failed test took 78.71 seconds overall versus 25–28 seconds in those passing runs. A failure-only bounded diagnostic now records the captured claims, budget, blockers, nodes, elapsed time and separately labeled post-failure fake attempts/marker states; the assertion and timing are unchanged. Independent review found no diagnostic privacy or correctness blocker. The finalized diagnostic variant passed three consecutive Windows ARM64 local runs in 89.450 seconds. Subsequent exact-source hosted runs passed this scenario, but no diagnostic established the original cause; that uncertainty remains in acceptance.md.

An independent reservation audit found the legitimate owner serializes capacity reads and inserts in a single SQLite write transaction, with exact claim release following verified cleanup. Added `internal/store/shared_account_concurrency_test.go`: two concurrent fake attempt claims sharing one declared account produce exactly one winner; the other remains queued until verified not-launched settlement releases the claim. Ten local repetitions passed. The independently reviewed test covers concurrent store callers and serialized atomic admission, not parallel SQLite writers, physical process overlap, or the unknown cause of the Linux CI observation.

## Exact-source CI qualification and independent exception review

Exact-source run 42454538785 at `8d1bc3a8c598df305f67dab160a774a18c79a026` reports completed/success overall and six individual job successes. Darwin AMD64 job 128650643743 has all 15 steps completed/success and a complete log, including build, full tests, checks, release preview and cleanup, but its job/check API status remains in_progress without a conclusion. Independent review inspected the API and log and found that this narrow combination establishes execution of the required native checks; the explicitly reasoned equivalent gate is in plan.md. The missing individual conclusion remains an unresolved metadata discrepancy for 012 and is never counted as an individual job success. The Linux mount namespace test was skipped. The prior intermittent Linux ARM64 shared-account fixture failure remains of unknown cause; later passes and the separate store concurrency test mitigate but do not explain it.
