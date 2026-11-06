# Verified delivery design review

Status: source design and read-only Specify analysis CLOSED, subject to the immutable design
commit before runtime implementation. This is not implementation or acceptance of005.

Root owns plan/contracts. Domain/store reviewer authored data-model.md; native/CLI reviewer authored
research.md; UI reviewer authored quickstart.md. Each reviews foreign artifacts, with Root reviewing
their owned documents. No author approves their own implementation or evidence.

| ID | Severity | Finding | State |
| --- | --- | --- | --- |
| DR005-01 | P2 | Native review/check declarations needed exact closed public DTOs separate from captured server IDs/hashes. | Root added concrete CheckDeclaration and Review DTOs; native actual-source recheck closed it. |
| DR005-02 | P2 | Current-only workflow reads hid historical workflow relationships after guidance/adoption. | Explicit historical read selectors and bound cursors are consistent in model/contract/quickstart; native recheck closed it. |
| DR005-03 | P2 | An immutable step's one attempt could not also own a004A replacement; competing004/005 advancement was ambiguous. | New immutable replacement execution preserves slot/input, consumes only004A proof and prevents duplicate advancement; qualifying success/correction triggers are distinct. Native actual-source recheck closed it. |
| DR005-04 | P2 | Known conditionKeys had no canonical source for original/free-form conditions and check declarations. | Whole-original/condition registry and canonical digests are explicit; no model-derived split or waived condition. Native actual-source recheck closed it. |
| DR005-05 | P2 | Builtin verifier identity differed between model and contract. | Canonical literal_document_replace/1 is aligned and independently rechecked by the native reviewer. Closed. |
| DR005-06 | P2 | Default Auto and supporting attempts could break the existing exact response-only user acceptance or display review JSON as the answer. | Selected author/correction response binding and existing user acceptance preserve standalone/replay and conjunctive evidence. Native actual-source recheck closed it. |

All reported source-shape/authority findings above are corrected in the actual artifacts and
independently rechecked. Root independently inspected the foreign data-model/research/quickstart:
anchored exclusive moves, metadata eligibility, same-volume limits, live quarantine retention,
executor-specific evidence and prospective scope agree. Native and UI readers reviewed foreign
Root contracts/plan; no author approves their own artifact. The durable per-effect permit keeps
Stop responsive without a fictitious filesystem rollback or new effect after cancellation.
A further native compatibility check confirmed the actual Codex exec strict schema requires all
properties required and additionalProperties:false in every object. The contract now uses required
inspection paths arrays and positive location ranges; no optional/null schema branch or generic
schema framework is needed. Native primary documentation and exec interoperability tests support
this shape; live behavior remains unqualified.
Read-only Specify analysis ran its required prerequisite once after the complete task file existed.
It found35/35 VFD/SC covered,59 unique tasks,15 parallel authoring tasks, zero unmapped tasks or
unknown IDs, zero cycles, constitutional conflicts, critical/high issues, placeholders or duplication.
Three task-only findings were corrected outside the read-only analysis: T011 now names the
actual domain version constants and package-lock with sole-writer coordination; T007 says nine
tables plus indexes; T057 explicitly pushes the checked source before hosted qualification.
The independent analyst re-read those three actual changed task lines and closed I1/I2/U1,
without rerunning prerequisites, editing source or claiming runtime qualification. The immutable
commit containing this reviewed artifact set and T001 closes the implementation gate.
No implementation or passing runtime is implied.

## Exclusive implementation ownership at the design gate

- Core: all new internal/domain/workflow*.go and required existing domain execution/validation/types
  changes (including authoritative dev.5/005 constants); all store workflow/delivery/schema/tasks/
  attempts/results/fingerprints/cursors and their assigned tests. Publish T004 DTO/controller/store
  signatures before other lanes bind source. Core is the sole existing domain/store writer.
- Native: internal/workflow/ pure policy/validation; new review adapter and specifically necessary
  existing adapter changes; workspace handoff/application/platform helpers and their assigned tests;
  internal/fixture/workflow.go. No process/privacy/evidence infrastructure or CLI dispatch edits.
- UI/CLI: authored UI, browser tests/helper and UI api.ts current/maintenance mirror; new
  internal/cli/workflow*.go plus specifically assigned new CLI tests. Existing CLI dispatch/parser/
  execution/metadata remains Root-owned. Coordinate exported entrypoints before shared hooks.
- Root: engine/app/HTTP integration and tests, existing CLI hooks/metadata, manifests/package scripts,
  generated dist, sole heavy build/test/browser queue, final evidence and Git. No direct vendor calls.

Only the active005 feature is implemented. Tests/migrations/scenarios use local fixtures and private
state. Schema/DTO publication precedes dependent activation. One writer per shared file; further
assignments require explicit handoff. No code begins until the design commit succeeds; no006 code
begins before005 independent correction, complete local/native/race qualification and acceptance.
