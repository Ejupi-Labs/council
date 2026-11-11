# Post-acceptance review of the `refactor` branch (2026-11-11)

Scope: a read-only code review of the accepted features at `5eb75bf` (MCP host `internal/mcphost`,
`internal/ownerclient`; history and recovery `internal/historyout`, `internal/store`; UI `ui/src`),
plus a ledger consistency pass. This record does not accept, reopen or extend any feature, and it is
not hosted-CI or live-provider evidence. Reviewers were read-only subagents; each correction below
was checked by reading the code before it was changed. Nothing here has been committed or pushed.

## Environment limits

- Windows host, Node 24.14.1 (below the required 24.18.1), Go 1.27.3.
- No symlink privilege (unelevated account), so symlink-dependent Go tests skip.
- Playwright Chromium is not installed, so **no browser (`ui/tests`) test was run**. The UI
  corrections and the new regression test below are type-checked and built, not executed.

## Observed local results

| Check | Result |
| --- | --- |
| `npm run check` (go vet, tsc, format) | Exit 0, before and after the corrections |
| `go test -p 1` of every non-engine package, first run | All `ok` except `internal/localstate` (see correction 1) |
| `internal/localstate` after correction 1 | `ok` |
| Engine package, 26 case batches (run before the dead-code removal in correction 6) | 26/26 passed |
| `node --test scripts/*.test.mjs` (build, package, offline-install) | 3/3 passed |
| `internal/store` and `internal/localstate`, after correction 6 | `ok` |
| `npm run build` | Built; `internal/webui/dist` changed only for `api.js`, `routing.js`, `tasks.js` |
| Browser tests (`ui/tests`) | **Not run** |
| Engine batches after correction 6 | **Not rerun** (only unused methods were removed; `go build ./...` and `go vet ./internal/store` are clean) |

## Corrections made

1. `internal/localstate/localstate_test.go`: `TestRejectRedirectedPrivateFileTargets` failed with
   `t.Fatal` when the host cannot create symlinks, while its sibling `TestRejectRedirectedRoot` skips.
   It now skips the subtest with the OS error. Fixture robustness only; no product behavior changed.
2. `ui/src/api.ts` `consumeAccessToken`: any non-`token=` fragment was stored as the access token.
   Clicking an in-page link (`#agents`, `#tasks`) and reloading overwrote the real token and caused
   401 on every call. Only `#token=` fragments are consumed now; navigation fragments are left alone.
   Regression test added to `ui/tests/workspace.test.mjs` (**unrun**).
3. `ui/src/tasks.ts`: switching task or project (or opening a task without executions) left the
   previous task's execution summary, facts, events, artifacts, native-model panel and pagination
   cursors visible and usable. New `clearAttempt()` empties them. The previous task's attempts
   cursor could also be sent for the new task.
4. `ui/src/tasks.ts` `mutate` and `ui/src/routing.ts` `amend`/`adoptAndStart`: a late reply for
   task A no longer writes its status into task B's status region. The pending operation is still
   retained and shown when A is reopened.
5. Stale status banners and one contradiction: `delivery-plan.md` ("001–006 accepted; 007–012
   planned"), `specs/007a-metadata-history/{review,spec,data-model}.md` and
   `contracts/operations.md`, `specs/008-host-integration/{spec,quickstart}.md` and
   `contracts/catalog.md`. Original wording is preserved in the new status lines; the 007a review
   has an appended closure note that cites `acceptance.md` and adds no new evidence. The history
   contract now states the real filename rule (`history-<name>.json`) and that the file is created
   in the private state root.
6. Dead code left by the cumulative migration chain, removed with `go build`/`go vet` clean:
   `validateExecutionSchema`, `validateRoutingSchema`, `validateWorkflowSchema`,
   `validateGraphSchema` and `graphBudgetEpoch` had no callers (open-time validation runs through
   `validateHistorySchemaWith`). A migration comment now says "schema 6 to 7".

## Findings reported, not fixed

These need a design decision or a spec/review cycle rather than a local patch. None was reproduced
with a failing test unless stated.

**History and recovery (P2).**

- A history export left `uncertain` or `effect_started` (for example ENOSPC or an unsupported
  directory fsync) makes every later clean owner shutdown incomplete. The next start records a
  global `owner_epoch_uncertain` dispatch fence, and no reconcile path can clear it
  (`internal/store/recovery_owner_epoch.go:209`, `history_reconcile.go:399-401`). Unrelated task
  dispatch stays blocked. Confirmed by reading; `TestHistoryOwnerEpochRetainsUnsettledPublication`
  asserts the dirty epoch but nothing offers an exit.
- A paused scope can become unresumable: guidance edited after pause, an accepted Stop before
  pause, or an `incomplete` native evidence blocker leaves resume returning
  `checkpoint_scope_conflict` / `scope_stopped` or the checkpoint stuck `pausing`; checkpoint
  states `cancelled` and `failed` exist in the schema but are never written
  (`internal/store/recovery_resume.go:71,459-474`, `recovery_checkpoint.go:142-146`).

**History and recovery (P3, mostly plausible/unreproduced).** A resumed graph member can be admitted
after a budget grant without an explicit advance (`graph_input.go`, `graph_grant.go`); a fresh
database failing between v8 migration and epoch initialization leaves a permanent
`owner_epoch_uncertain` fence (`store.go:98-149`); reconcile on a permit-present history operation
always returns `unresolved` yet appends journal rows toward the 4096 bound; the persisted
`RootIdentity` embeds the Linux statx mount ID; a history preview is not single-use; reserved
history/recovery states and unused exported helpers are reachable from shipped code; Windows maps
some existing-target ACL/reparse cases to `ErrUnsupported`/`request_interrupted`.

**MCP host (no P1; invariants for stdout, subprocesses, 1:1 catalog mapping and observer
cancellation held).** P2 journey gap: no MCP tool can read an older owner's `ownerInstanceId`, so
`council_service_stop` for a recognized older owner cannot be driven from MCP alone. P3: non-Stop
calls against an incompatible live owner stall about 10 s before `owner_unavailable`; a cancelled
request is still answered; some advertised schemas lack `maxLength` (validators still bound them);
unknown members in non-`tools/*` params are dropped by the SDK; SIGINT on a real blocking stdin may
delay shutdown; `Envelope.Data` has `omitempty` on a v2 encoder (latent, not reachable today);
the CLI `forward` still decodes `data` into `any` (all current 64-bit public values are strings,
so not reachable today).

**UI.** P2: a retried mutation whose first attempt was dispatched never leaves "pending" even on a
definitive 4xx verdict (`workspace.ts`, `drafts.ts`, `tasks.ts`, `routing.ts`, `workflow.ts`,
`recovery.ts`, `history.ts`, `graphs.ts`). Changing it needs care because an idempotency conflict is
also a 4xx and does not prove non-commit. A pending Start is not scoped to its project and is
labelled only "Retry pending start". Late *error* paths in `routing.ts` `mutate` and `workflow.ts`
`perform` still write to the shared status region. P3: workflow evidence disclosure is rebuilt on
every poll; live regions are rewritten with identical text; "Start another execution" takes its
mode from the composer checkbox for tasks without workflow authority; repeated accessible names in
the graph editor; list rebuilds drop focus; type drift and duplicated validators and helpers
(`timestamp` validators disagree); copy exposes spec numbers and raw tokens.

**Remaining documentation drift.** `design-identity.md` says "Not reported" where the UI and
constitution say "unknown"; `specs/001-workspace-foundation/spec.md` US3 AC3 still says execution is
unavailable in this preview; `specs/007a-metadata-history/{spec,plan,data-model}.md` describe a
"private export directory" that does not exist (the file is written into the state root);
`specs/008-host-integration/plan.md` says the owner client is shared with the CLI "where
practical" while the CLI keeps its own `forward`.

## Open roadmap (unchanged by this review)

Full scoped evidence export, production backup/restore and reviewed cleanup (`specs/007-scoped-retention/`,
a draft awaiting a payload-privacy decision) and features 009–012 remain unimplemented and unaccepted.
