# Economical routing implementation review

Status: CLOSED — independent source/correction review, local qualification and all six native
plus Linux race jobs passed at the exact source below. This is not completion of the rebuild goal.

The design gate was committed at `e9b37ad` before runtime changes. Reviews below inspected that
baseline plus the working implementation. The accepted 003 source remains `ce8aaa7`. No vendor
credentials, administrative probes or live provider tasks were used. Tests use local fixtures.

## Independent scope

- The native/CLI author independently reviewed the domain/store transaction and continuation
  implementation, then the root's launch/mutation integration and the separate read/HTTP lane.
- The separate read/HTTP author reviewed foreign routing, CLI and UI source; their own four
  implementation files were excluded from that review.
- The domain/store author independently reviewed the root's preparation ordering, original and
  managed source checks, project identity and continuation boundary.
- Root inspected the store, routing, interface, fixture and packaging boundaries, ran the shared
  checks and owns the final evidence gate. UI rendering receives separate observed inspection.

## Corrected findings

| ID | Severity | Finding and correction | Verification |
| --- | --- | --- | --- |
| IR004-01 | P2 | Budget arithmetic assigned a successful typed-nil validation error into an error interface, causing a false rollback/panic. Use separate typed validation locals. | The failing budget case was reproduced; the full store suite subsequently passed. |
| IR004-02 | P2 | A literal source file could be replaced with identical bytes after its initial physical proof check. Recheck original identity after Capture, before managed proof and preparation. | Actual resolver barrier red/green; independent native source recheck closed it. |
| IR004-03 | P2 | Eligibility mutations could commit between the durable guard and native Prepare. Order them with prepareGate → control, releasing control during Prepare and keeping Stop/Shutdown control-only. | Actual preparation barrier and Stop acknowledgement/cancel tests passed; independent review closed ordering. |
| IR004-04 | P2 | A project directory could be replaced after its saved identity check, then captured under the former identity. Revalidate saved project identity after Capture, before source registration or Prepare. | Two disposable Git-root swap red/green; independent store-author recheck closed it. |
| IR004-05 | P2 | Duplicate account/context report projection erased a non-null inherited link provenance while the link remained active. Preserve the context's inherited provenance when merging a nil account-origin copy. | The real browser found it; strengthened linked and unlinked store regression red/green. |
| IR004-06 | P2 | Configuration refresh reset an unsaved explicit empty Auto guidance selection to Keep. Preserve a valid current option by membership, including empty. | Delayed actual browser refresh and exact guidance payload passed; independent actual-source correction recheck closed it. |
| IR004-07 | P2 | A shared economical Auto label retrospectively changed 003 requested/history meaning. Use neutral no-literal-pin wording in shared historical facts and economical copy only for current selection. | Actual populated-schema3 adoption/history assertions passed; independent actual-source correction recheck closed it. |
| IR004-08 | P2 | Optional explicit-empty wire enums, IDs, counts and timestamps disappeared into Go omission defaults. Add strict request-presence rules while retaining valid empty provider/note text and false. | Nineteen raw-object cases plus valid controls; focused domain/store checks passed. |
| IR004-09 | P2 | CLI history URLs used flat suffixes instead of the frozen nested paths. Correct both mappings and expected contract URLs without adding aliases. | Focused CLI mapping check passed. |
| IR004-10 | P2 | CLI rejected the documented empty provider-label/note values. Permit those intentional text values while rejecting empty routing choices. | Meaningful parser red/green and preserved empty provider update pointer. |
| IR004-11 | P2 | Standalone CLI price files bypassed nested request-presence checks. Share reference-price presence validation between the standalone and nested DTOs. | Domain and actual CLI-file regressions passed; independent source recheck closed it. |

DTO review also corrected the source enum and wire tags, kept prior report JSON immutable and
replaced repeated full capacity views with at most two compact immutable context evidence
snapshots. Decisions validate their encoded bound before commit. These are contract corrections,
not new execution capabilities or extra user settings.

## Concurrency and authority notes

Ordinary global occupancy remains queue waiting. Only a supported known pre-launch negative can
produce category-A proof; unknown installation or lost positive coverage can block a target but
does not authorize automatic replacement. Category B remains unsupported. A fresh settled Stop
can suppress continuation by advancing task control revision without rewriting its terminal or
proof. A Stop aimed at an older predecessor does not stop an already admitted successor.

Automatic admission consumes one immutable predecessor once, within the current owner epoch and
matching task/version/budget revisions. Previous price/tier comparisons use captured decision
evidence. The engine calls continuation only after a fresh invocation returns, including deferred
reservation cleanup; it never scans old terminal rows on restart or a configuration refresh.

## Fixture corrections and remaining gates

Several qualification failures were in fixtures: typed-nil test shadowing, unordered JSON map
comparison, ambiguous nested summary selectors, an interceptor consumed by GET instead of POST,
and assertions expecting internal error codes in human UI text. Corrections retain actual HTTP
code assertions and recovery text. A source-replacement fixture initially resolved the test
executable and timed out in its own contained child; it was replaced with a controlled invalid
native file before the recorded regression, so a broken guard cannot execute any program.

The full CLI run exposed old synthetic schema1/2 fixtures retaining new routing tables during
downgrade. Fixture repair is separate from production's required incompatible-schema refusal.
No production rejection or migration boundary is relaxed for these tests. Both corrected legacy
CLI cases subsequently passed in 27.323s, and an independent native-author recheck closed the
fixture scope. Actual QA produced 28 captures, 15 keyboard checks and ten matching asset hashes;
root and a second reviewer opened separate desktop/mobile selections without a visual blocker.

Final local `npm test` passed: all Go packages (unchanged packages validly reused test cache),
39/39 browser cases in 129.728s and 1/1 extracted-package case in 9.849s. The final complete store
run included the additional replacement/escalation/captured-price/no-rearm authority tests and
passed in 4.146s. Final `npm run check` also exited 0. The final test command rebuilt the native
binary and embedded UI. Actual visual/accessibility inspection and asset comparison are recorded
above and in acceptance.md. No known P1/P2 source finding remains open.

The native/CLI author independently inspected the final authority-test addition: each refusal
first establishes the intended captured predecessor, a real durable A proof and an eligible
alternative, then checks no admission/budget/ledger effect. Allowed equal/lower controls prevent
blanket-denial false passes. Captured-price and explicit-Start controls were also inspected.
No P1/P2 remained; this review claims store authority coverage, not native execution by that test.

The reviewed source is committed as `3f4a18e11de495f7b3138d61e030525a018868a5`, including matching
embedded assets. All six actual native platform jobs and Linux race qualification passed in
run 42172669381; Root confirmed exact source/terminal jobs and the independent native/CLI reviewer
read all seven logs. Each native log reports browser39/39 and package1/1 with no failures,
cancellations or skips, plus clean generated assets. The 38-row matrix and acceptance.md close
the feature gate without a live-provider claim. Features005–012 remain outside this implementation.
