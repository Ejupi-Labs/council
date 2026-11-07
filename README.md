# Council

A local workspace for orchestrating coding agents across harnesses and providers.

This is the **refactor** development branch. The clean rebuild is being delivered one specified
feature at a time. The accepted functional reference is preserved under [reference/legacy](reference/README.md);
main remains unchanged.

## Current preview

Preview **2.0.0-dev.6** implements bounded parallel and dependent task groups on the existing
native Codex/Claude Code workflows. Features 001–006 are accepted for their documented offline
scope; feature 006's exact-source native and race qualification is recorded in its
[acceptance record](specs/006-task-graphs/acceptance.md). Auto connects authoring, proportionate
checks, necessary independent review, bounded correction and captured delivery. The Ejupi Labs UI
and CLI retain original instructions, literal pins, stage lineage, exact output and history.

Feature 007, durable pause and safe recovery, is accepted for its documented offline scope at
product revision `0a963e8`. Its [acceptance record](specs/007-recovery-and-retention/acceptance.md) and
[recovery guide](docs/refactor/recovery-guide.md) describe the evidence and limits. Scoped retention
and the remaining host, harness and release work follow in the
[delivery plan](docs/refactor/delivery-plan.md). Native provider qualification requires a separate
explicitly requested live task and remains unperformed.

Start with a project and task. No account, quota, price or model form is required. Auto uses a
compatible native default for unknown suitability. An explicit routine/general hint or a narrowly
recognized document replacement can prefer a suitable declared profile. Optional user reports can
exclude busy Codex capacity and route new work to eligible Claude. Dev.6 permits up to two managed
workers across eligible native contexts. A context, shared-account reservation or narrower graph
limit can reduce that capacity; a held migrated legacy claim remains a sole dispatch barrier until
settled. Queued work keeps its admitted target. Existing personal chats and vendor settings are unchanged.

The fixed profiles are:

| Profile | Literal native selector | Declared policy role |
| --- | --- | --- |
| codex-native-default | Native configured default | Unknown suitability |
| codex-luna-economy | gpt-6-luna | Economy |
| codex-sol-general | gpt-6.1-sol | General |
| claude-native-default | Native configured default | Unknown suitability |
| claude-haiku-economy | haiku | Economy |
| claude-sonnet-general | sonnet | General |

Roles and selectors are dated declarations, not measured quality, effective-model identity,
entitlement or savings. Claude's documented transport minimum is 2.1.259; Haiku/Sonnet alias
assumptions are 2.1.293/2.1.284. Unknown installed versions stay unverified. Native provider
compatibility remains **unqualified** for both connections. Discovery checks filesystem metadata
without invoking vendors or inspecting credentials. Fixed fake mode exercises real process
ownership, workspace and evidence handling with local fixtures; it never falls back to a provider.

A fresh installation retains the005 default of **four managed model admissions in one shared 30-minute
wall-clock window** from the first admission. Every existing saved default, including two, stays
unchanged. Existing task grants, use and deadlines are never refilled by an upgrade or preference
edit. Author, reviewer and corrector processes consume model admissions; local checks and local
application do not. Queue, preparation, execution and required settlement share the captured
window. Cleanup can continue after expiry, and a late applied effect does not turn a failed task
into success. An explicit grant adds finite authority without starting work or re-arming an old
automatic result.

Routing continuation defaults OFF; its bounded opt-in supports only proven pre-launch ineligibility
before native process preparation/creation. Auto workflow correction is separate: at most one
captured cycle can follow an actual review blocker or observed failed required check within the
remaining task authority. Generic native errors, prose and uncertain cleanup authorize neither
retry nor correction. Native internal calls, retries and fallback remain outside Council's
managed admission count.

Usage views keep known sourced counters, missing coverage, fake work, reference-price declarations
and native client estimates separate. Declared accounts/links group reports and policy; they do
not select or attest native login identity. Actual model/account/access/allowance and charges stay
unknown when unreported. Paid API execution, actual-account pins, hard spending caps and exact
effective-model guarantees are unsupported. No subscription saving is inferred from a fixture.

Ordinary Start uses Auto without an account, model or workflow wizard. A narrowly recognized
literal document replacement can finish with one author and a sufficient observed builtin check.
Other work retains the necessary check/review gaps and, when authorized, linked correction and
recheck evidence. Agent claims do not replace observed verifier receipts. Optional
`--workflow author_only` retains standalone authoring without a qualified Auto workflow result.

Delivery defaults to review only. Saved autonomous full access does not grant local delivery:
choose the prospective verified-local scope once to authorize compatible future tasks. Under that
captured scope, ordinary single-link text additions, replacements and deletions under existing
parents can be applied while preserving unrelated working and staged edits. Unsupported index,
mode, type, symlink or cross-volume changes remain retained with a named gap; destination conflicts
preserve the candidate and user work. Candidate creation records stay immutable; current workflow
evidence and acceptance describe later qualification and delivery.

Commit, push, publication and deployment are not implicit. A supported external command needs an
explicit task declaration with its action, destination and arguments. Its successful command exit
does not attest remote publication. Plain response-only work keeps the existing explicit response
acceptance action and exact replay; supporting review never replaces the selected author's answer.
Native session IDs are observed identifiers, not resume support or Council conversation history.

## Task groups under qualification

Create an optional group of up to16 required nodes and64 file/context dependencies in one project.
The complete UTF-8 JSON declaration is limited to2MiB and validated before execution. Standalone
Start remains Auto. Groups default to **Separate reviewable results**, at most two concurrent
workers and one captured finite allowance/common window. Authors, reviews, corrections and retries
all spend from that root allowance; nodes and guidance never acquire separate grants.

File dependencies reconstruct exact accepted predecessor files and staged content from retained
evidence in the receiver's isolated workspace. Context-only edges supply accepted evidence without
implying files. Compatible fan-in preserves shared ancestry once; divergent file/index/type changes
block with retained results. A successful process or an output path alone does not qualify input.

Focused context keeps graph-wide constraints, receiver instructions and relevant contributed
conditions with exact original references. Derived digests are labelled. Missing access, corruption,
more than64 supplied references or a combined prompt over256KiB blocks instead of clipping required
material. These are local transport bounds, not a provider token-capacity claim. Status, waiting,
materialization and default context construction add no model admission.

Only a prospectively designated final node may deliver. It must incorporate every required file
branch through file edges and qualify the cumulative graph-root-to-final change with current checks
or review before application. Intermediate outputs remain reviewable; they are not applied twice.
A response-only summary cannot authorize an unqualified file delta. External commands still need
explicit final task scope; command completion does not attest a remote effect.

```text
council graphs start --project PROJECT_ID --graph-file graph.json --request-id START_REQUEST_ID --json
council graphs get GRAPH_ID --json
council graphs budget GRAPH_ID --json
council graphs inputs GRAPH_ID --json
council graphs dispatches GRAPH_ID --node final --json
council graphs guide GRAPH_ID --node final --guidance-file guidance.txt --request-id GUIDANCE_REQUEST_ID --json
council graphs advance GRAPH_ID --request-id ADVANCE_REQUEST_ID --json
council graphs advance GRAPH_ID --rebind-node final --request-id REBIND_REQUEST_ID --json
```

Use the returned IDs and a distinct request ID for each intended mutation. With proof flags omitted,
the CLI captures current graph/node/window authority once before sending and retains that exact
request for retry. Supply every flag in a proof group when using explicit proofs; partial proof groups
reject. Retry with the same request ID and unchanged input rather than refreshing uncertain intent.
See the [task graph quickstart](specs/006-task-graphs/quickstart.md) for complete declarations and controls.

Guidance creates a new version without starting work. Advance deliberately admits eligible unfinished
work within existing authority. An explicit named rebind can append a `dependency_rebind` version
for changed accepted inputs; it preserves originals, options and prior evidence without refilling
allowance. Omitted rebind IDs and ordinary Run never silently rewrite an existing workflow or replay
completed work. A grant alone starts nothing. Stop group cancels the group's admitted work; Stop node
cancels its own branch and blocks unstarted descendants while independent admitted siblings settle.
After receiver first GO, ancestor Stop alone does not retcon its captured inputs; changed guidance,
its own Stop and group Stop still bind. Restart reads history without replay; fresh Advance is explicit.

## Run a native preview

Extract a platform package and run its `council` executable (`council.exe` on Windows). Release users
need no Node, npm, Go or compiler. Git is required for project inspection/source capture; a browser
is needed for UI. Native work needs the chosen supported native executable and existing vendor
login. Shell wrappers are not launched.

```text
council ui
council projects open /absolute/path/to/project --json
council agents list --json
council drafts create --project PROJECT_ID --task "Describe the work"
council tasks start --project PROJECT_ID --task "Describe the work" --json
council tasks start --project PROJECT_ID --task "Routine local work" --suitability routine --json
council tasks start --project PROJECT_ID --task "Describe the work" --harness claude-code --json
council tasks start --project PROJECT_ID --task "Local fixture" --fake --json
council workflow preferences get --json
council tasks workflow TASK_ID --json
council tasks steps TASK_ID --json
council tasks checks TASK_ID --json
council tasks reviews TASK_ID --json
council tasks findings TASK_ID --json
council tasks deliveries TASK_ID --json
council routing contexts --json
council routing capacity --context codex-native --json
council tasks routing TASK_ID --json
council tasks budget TASK_ID --json
council tasks usage TASK_ID --json
council attempts get ATTEMPT_ID --json
council attempts stop ATTEMPT_ID --json
council service stop --json
```

On Windows supply a native absolute path, quoting spaces normally. `council ui --no-open --json`
reports a private local launch URL instead of opening a browser. Keep that URL private. Closing
the browser ends observation while work continues. Foreground Ctrl+C requests owner shutdown;
explicit CLI execution starts a continuing owner when needed. `service stop` targets the existing
owner and settles its work. Current execution requires dev.6, execution/workflow/graph contracts
`006-v1` and the current capabilities. Exact known dev.2/dev.3/dev.4/dev.5 owners retain only
their maintenance Stop compatibility path; they do not grant current execution compatibility.

New state lives at `<user-home>/.council/v2/`, separate from legacy collaboration data. Retained task
instructions/history and acknowledged receipts keep their original meaning. Legacy tasks require
explicit budget adoption before a new attempt when that authority is missing. Old tasks remain
standalone until explicit workflow adoption creates a new captured version. Neither adoption
starts work: use **Run task** for an adopted workflow's first author; advance only an existing
eligible workflow stage. Autonomous full access is the requested default and maps to supported
native permission controls. Supervised
approval is unavailable, and no universal OS sandbox is promised. Draft updates and routing
mutations use expected revisions. Preserve `--request-id` when retrying an uncertain mutation.

## Develop

Prerequisites: Git, Node24.18.1+ and Go toolchain auto-download support. Go1.27.3, TypeScript7.0.2
and browser tooling are pinned. Runtime code has no npm dependencies.

```text
npm ci
npx playwright install chromium
npm run build
npm test
npm run check
npm run release:local
```

Build emits `build/council[.exe]` and embeds the compiled browser assets. Tests use disposable Git
projects, fake installations and local browser fixtures; they make no live agent calls. Native
binaries and runtime state are excluded from Git. Compiled browser assets are committed with source.
Platform runtime evidence and cross-compilation evidence are reported separately.

## Product and delivery documents

- [Vision and scope](docs/refactor/vision.md)
- [Functional requirements](docs/refactor/functional-requirements.md)
- [Functional specification](docs/refactor/functional-specification.md)
- [Ejupi Labs design identity](docs/refactor/design-identity.md)
- [Ordered feature delivery](docs/refactor/delivery-plan.md)
- [Requirements traceability](docs/refactor/traceability.md)
- [Workspace foundation specification](specs/001-workspace-foundation/spec.md)
- [Managed execution specification](specs/002-managed-execution/spec.md)
- [Managed execution quickstart](specs/002-managed-execution/quickstart.md)
- [Claude interoperability specification](specs/003-claude-interoperability/spec.md)
- [Economical routing specification](specs/004-economical-routing/spec.md)
- [Economical routing quickstart](specs/004-economical-routing/quickstart.md)
- [Verified delivery specification — accepted native/offline scope](specs/005-verified-delivery/spec.md)
- [Verified delivery quickstart and qualification procedure](specs/005-verified-delivery/quickstart.md)
- [Task graph specification — implementation under qualification](specs/006-task-graphs/spec.md)
- [Task graph quickstart and remaining gates](specs/006-task-graphs/quickstart.md)

Each feature follows Specify specification → design → tasks → implementation → independent review
→ corrections and acceptance before the next begins. Technology and limitations are documented in
the active feature's design. No release or model-saving claim is inferred from a mock agent response.

## Licensing

Council source is MIT. Ejupi Labs generic icons retain their MIT notice. Company identity artwork
has separate provenance and is not relicensed by the icon package. See [asset provenance](third-party/brand-assets.md)
and the notices included with native preview packages.
