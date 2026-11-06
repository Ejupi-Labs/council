# Implementation Plan: Claude Code interoperability

**Branch**: `refactor` | **Feature context**: `003-claude-interoperability` | **Date**: 2026-11-06

**Spec**: [spec.md](spec.md) | **Input**: independently reviewed 21 CLA requirements and 8 success criteria.

## Summary

Add a second native harness through the accepted lifecycle. Reuse isolation, immutable task intent,
process ownership, input delivery, evidence, cancellation, response acceptance and no-replay
recovery. Extract the small parser/launch boundary; choose Codex or Claude by the captured harness.
Claude final text comes from its validated result event, while Codex retains its reserved output
file. Keep one global managed-native slot and the existing Codex Auto default until feature 004.

The prerequisite is accepted implementation f5edd96cbe32a1cb78d01d9b1107205c5abd8a00: all six native
platform jobs and Linux AMD64 race checks passed in [run42140818152](https://github.com/Ejupi-Labs/council/actions/runs/42140818152).
Do not repeat its resolved process or storage architecture as a second implementation.

## Technical Context

| Concern | Choice |
| --- | --- |
| Language/runtime | Existing Go1.27.3, authored strict TypeScript embedded UI |
| Dependencies | Existing modernc SQLite1.61.1/SQLite3.54.1 and x/sys0.49.0; no new runtime dependency or vendor SDK |
| Tooling | Node24.18.1, TypeScript7.0.2, Playwright1.62.0; development only |
| Product shape | Local single-user native CLI/loopback UI; host integration remains feature008 |
| Targets | Windows/macOS/Linux, AMD64/ARM64, plus supported Linux AMD64 race qualification |
| Preview/contract | Application2.0.0-dev.3, API1, executionContract003-v1, transactional schema3 |
| State | Existing private user-home/.council/v2; no legacy state access or new state namespace |
| Native profile | Claude native print/text input/stream-json with verbose output, no saved session, explicit bypassPermissions and noninteractive denial of unhandled prompts |
| Interface assumptions | CLI2.1.259+ for permission-prompts none; current official documentation/release2.1.318 observed2026-11-06, not live binary qualification |
| Capacity | One global managed-native pool shared by Codex/Claude and fake/native attempts |
| Performance | No model calls for observation/selection/admission; one bounded worker, existing1800s default deadline and15s termination budget |
| Scale | One local owner and existing100-entry pages; no account scheduler, hosted service or distributed graph |
| Limits | Existing prompt256KiB, stdout64MiB, stderr16MiB, JSONL1MiB, event16KiB and chunk64KiB; Claude final result must fit its single native message |

## Constitution Check

Pre-research and post-design obligations:

1. Useful outcomes: retain actual response/candidate and explicit remaining checks; authoring exit
   is distinct from task acceptance. No integration or invented review/verifier receipt.
2. Proportionate usage: one explicit worker, no automatic fallback/retry, no model for metadata or
   routing. Native internal work is reported only to the extent observed; its invocation is not a
   measured token/dollar cap. Suitable-target routing stays in feature004.
3. Honest interoperability: direct supported native executable, vendor-owned authentication,
   sourced model/settings/usage/version/session facts and declared unsupported capabilities. No
   SDK/API fallback, credential inspection, --bare or silent --safe-mode instruction suppression.
4. Autonomy: preserve the saved preference and native project instructions, apply supported native
   permission flags and report remaining denials. No extra per-tool approval loop or sandbox claim.
5. Evidence/recovery: use the same durable ownership, full-byte retention, ordered failure/Stop,
   uncertainty fences, exact receipts, source isolation and append-once terminals for both harnesses.
6. Small branded UX: one optional harness choice in the existing details/Agents flow, useful Auto
   default, English labels, approved icons and actual desktop/360px keyboard renders.
7. Specify sequence: specification review passed; this plan, research, data model, contracts and
   subsequent tasks must be reviewed and committed before application changes. No feature004 code.

No constitution exception is required. Native customizations can perform vendor-owned work; the
profile does not claim to suppress that work while preserving instruction/authentication semantics.
Supported boundaries and unknown telemetry remain explicit.

## Phase 0 — Research decisions

Use [research.md](research.md) for primary sources, dated live documentation and pinned supporting
SDK schemas. SDK code is evidence, not an imported runtime or live fixture capture.

- Use a direct native Claude binary and explicit argv/stdin. Do not run .cmd/.ps1/shell wrappers.
  Resolve documented native install locations and executable PATH candidates through metadata.
- Preserve ordinary native authentication and instructions. Do not add --bare or --safe-mode.
  Use print, text stdin, stream-json/verbose, no-session-persistence, bypassPermissions and
  permission-prompts none; append only the exact captured model selector when supplied.
- Keep version/interface assumptions separate from observed native version. Discovery stays
  metadata-only; no installed vendor help/version/auth/model call is part of development or refresh.
  A known incompatible prerequisite blocks; unknown compatibility stays unqualified and an explicit
  native attempt retains any actual startup failure without another account/harness fallback.
- Finalize Claude protocol only after EOF and owned process settlement. A supported one-shot run
  has one authoritative main result. Require subtype success and is_error false; a repeated or
  conflicting main result is an explicit unsupported/failed protocol outcome. Retain trailing
  observations; do not treat the result as a physical last-line or root-exit guarantee.
- Derive final_response from the decoded successful native result string, not assistant prose,
  partial deltas or tool output. Preserve raw stdout independently and its provenance link.
- Initial model is a reported setting. Use sourced actual main-assistant model evidence only when
  attributable/unambiguous; retain reported modelUsage separately. Requested aliases keep native
  selection semantics; extra background models alone do not prove a selector was substituted.
- Native usage and estimates keep their scopes: top-level usage, whole-query-tree modelUsage,
  client cost estimates rather than invoices. Provider/account/allowance/charged amounts stay
  unknown unless independently reported with adequate provenance. Never sum overlapping scopes.
- Retain the single pool; migrate only its claim representation. Preserve historical target labels,
  task/version JSON, receipts, artifacts, incidents and terminals exactly.
- Separate known-compatible exact-owner maintenance Stop from strict current execution handshake.
  Do not auto-stop an old owner, start a replacement to process Stop, migrate under another lock,
  delete a lock or infer authorization from a PID.

## Phase 1 — Design and contracts

### Shared adapter boundary

Introduce a minimal parser interface matching the current Write/Finish/Result/Failed behavior and
one harness dispatcher for launch/parser construction. Keep the existing Codex implementation and
its native tests. Avoid a registry framework, dynamic loading, another lifecycle controller or new
provider abstraction beyond the present two supported native choices.

The engine resolves an omitted harness as Codex, validates the captured target, then follows the
same preparation/claim/launch/control/settlement path. Claude does not allocate a Codex output
reservation. Its result text is published through existing immutable evidence.Put only after the
required native/drain/cleanup facts are established. Any publication uncertainty fences dispatch;
input delivery and terminal success remain independently enforced by the store.

The new Claude decoder accepts bounded strict JSON/Unicode and rejects ambiguous core fields,
duplicate members, malformed terminals and invalid essential numeric/protocol fields. Malformed
optional telemetry follows the explicit partial/unknown rules in the data model. Unknown additive observations
remain raw evidence, not new commands. Required protocol/state failures remain primary; documented
retry/progress observations are not fabricated terminal failures. A result carrying is_error true
cannot become success even with subtype success or root exit zero.

### State and compatibility

[data-model.md](data-model.md) defines concrete fields and validation. Schema3 replaces the claim
resource with a fixed managed-native pool token, carrying any exact outstanding attempt/owner
claim. Historical Attempt.connectionId remains codex-native; new attempts use their actual
codex-native or claude-native target. A capacity resource is never presented as an account quota.
Migrations preserve existing record/receipt bytes and atomically refuse inconsistent state.

Optional observation extensions identify configured model, native version and bounded actual-model
usage with source, time, scope and explicit preview completeness. The raw stdout artifact is the
full evidence reference when preview limits omit detail. Old observations retain the actual
observed/unknown wire states; no historical normalization is performed. Fake provider/model/account/
usage facts remain unknown rather than becoming measured provider observations.

[contracts/native-execution.md](contracts/native-execution.md) freezes HTTP/CLI metadata and target
selection, supported native profile, observation/response provenance, parser limits, one-slot rules,
receipt compatibility and older-owner Stop. New execution requires the current contract; only
explicitly known maintenance compatibility can reach a previous dev.2 owner. Lost-ack retries retain
the original bound owner, including when a replacement has since appeared.

### UI and developer commands

Add optional Codex/Claude choice alongside existing Auto details; ordinary Auto remains Codex and
says so until feature004. Agents can select the supported Claude target without introducing a
mandatory settings form. Capture the clicked choice in the existing pending start intent; later
selection/typing cannot change a retried payload. Existing task history and exact bytes remain.

Display actual captured harness, native continuity limits, version provenance, requested selector,
configured setting and reported model/usage scopes distinctly. Estimates are visibly estimates;
unsupported or absent data stays unknown. Keep summaries compact and expose full raw evidence
explicitly. Reuse phase-only list refresh, stale-generation guards, focus styles and brand assets.

### Source files affected

```text
internal/domain/       execution DTOs, capability/pin validation, preview/contract version
internal/adapters/     parser interface, Claude builder/decoder and harness dispatch
internal/fixture/      native-shaped Claude fake stream and failure/control scenarios
internal/store/        schema3 claim migration, shared pool and exact historical preservation
internal/engine/       target dispatch, response strategy, model/evidence observations
internal/discovery/    native Claude capability/launcher metadata; no vendor probes
internal/app/          existing execution route validation and metadata
internal/cli/          optional --harness, current handshake, known older-owner Stop
internal/httpapi/      existing strict routes/metadata contract; no arbitrary-path endpoint
ui/src/                optional harness selection and truthful native facts in current flows
internal/webui/dist/   compiled matching browser assets
scripts/               preview expectations and extracted fake/compatibility acceptance
specs/003-claude-interoperability/  feature design, tasks, reviews and acceptance
```

Process ownership, private state, evidence storage and workspace isolation are reused, not replaced.

## Validation and review strategy

[quickstart.md](quickstart.md) will describe executable fake journeys and exact evidence. Use unit
protocol cases plus shared lifecycle integration; do not test only emitted strings or build mocks.

- Validate Claude argv/prompt/policy/selector and metadata-only missing/native/wrapper discovery.
- Test real native-shaped JSONL successes, success+error, incomplete/malformed/multiple terminals,
  trailing observations, aliases, actual model sets, usage scopes and explicit output-limit failure.
- Apply existing source/index preservation, response-only acceptance, root/input/cleanup/evidence
  proof, storage-uncertainty, Stop/deadline and no-replay cases to Claude fixtures.
- Start different harnesses concurrently from two independent clients; prove one active worker,
  captured targets and ordered transfer only after verified settlement, including repeat receipts.
- Upgrade frozen synthetic schema1/2 records and exact receipt envelopes, including large counters,
  interrupted claims and incidents. Exercise known older-owner Stop with separate authenticated
  owner fixtures, lost replies, replacement owners and incompatible execution. No live task.
- Run actual UI/CLI Claude and mixed-target journeys, stale pending intent, keyboard/360px/desktop,
  model/cost/session labels and approved asset hashes. Render and inspect changed states.
- Run npm run build, npm test and npm run check; qualify native six-platform jobs plus supported
  race checks and extracted offline native package. Runtime evidence remains distinct from compile.
- Obtain independent source/failure-path review, correct blockers, recheck affected behavior and
  complete CLA/SC mappings before feature004 implementation. Record live qualification unperformed.

## Delivery and ownership

Freeze DTO/protocol/pool/maintenance contracts first. Then separate store/domain, adapter/fake and
UI work on explicitly owned files while root integrates engine/app/CLI and manifests. Keep edits to
shared files sequential. Root owns task status, Git, versioned acceptance and final gate decisions.

After design consistency review, commit specification, design and executable task list before
implementation. Do not add future account routing, parallel graph or integration scaffolds to make
this feature appear complete. No extension hooks or automatic Git-branch hooks are configured.
