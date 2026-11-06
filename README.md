# Council

A local workspace for orchestrating coding agents across harnesses and providers.

This is the **refactor** development branch. The clean rebuild is being delivered one specified
feature at a time. The accepted functional reference is preserved under [reference/legacy](reference/README.md);
main remains unchanged.

## Current preview

Preview **2.0.0-dev.3** adds explicitly selected native Claude Code alongside Codex in managed
isolated workspaces, with one shared execution slot. Claude interoperability has passed focused local
checks and independent review; full platform qualification is in progress. The accepted baseline is
feature 002. The Ejupi Labs UI and CLI share
versioned instructions, exact output, source-relative candidates, history, Stop and recovery
incidents. A response-only result can be explicitly accepted after review; authored changes remain
verification pending. Work is never automatically replayed, merged or published by this slice.

Native Codex and Claude live provider compatibility remains **unqualified**. Installation discovery does not
verify authentication, models, account capacity or subscription allowance. An explicit local fake
mode exercises the real ownership/workspace/evidence lifecycle without a provider. Economical
routing, further harnesses, independent verification/integration and MCP follow the feature plan.

## Run a native preview

Extract a platform package and run its `council` executable (`council.exe` on Windows). Release users
need no Node, npm or compiler. Git is required for project inspection; a browser is needed for UI.

```text
council ui
council projects open /absolute/path/to/project --json
council agents list --json
council drafts create --project PROJECT_ID --task "Describe the work"
council drafts list --json
council policy get --json
council tasks start --project PROJECT_ID --task "Describe the work" --json
council tasks start --project PROJECT_ID --task "Local fixture" --fake --json
council attempts get ATTEMPT_ID --json
council attempts stop ATTEMPT_ID --json
council service stop --json
```

On Windows supply a native absolute path, quoting spaces normally. `council ui --no-open --json`
reports a private local launch URL instead of opening a browser. Keep that URL private. Closing the
browser ends observation while work continues. Foreground Ctrl+C requests owner shutdown; explicit
CLI execution starts a continuing owner when needed. `service stop` targets the existing owner and
settles its work. Foundation metadata commands retain transient ownership when no owner is active.

New state lives at `<user-home>/.council/v2/`, separate from legacy data. Council does not inspect
vendor credentials or run vendor executables during installation discovery. Autonomous full access
is the requested default and maps to native supported permission controls. Supervised approvals
are unavailable in this slice. Auto model/account facts remain unknown unless the harness reports
them; Council does not promise a universal OS sandbox or a native spending cap.
Draft updates require an expected revision. A failed/uncertain save preserves its request identity
for reconciliation; use `--request-id` when retrying CLI mutations.

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

Each feature follows Specify specification → design → tasks → implementation → independent review
→ corrections and acceptance before the next begins. Technology and limitations are documented in
the active feature's design. No release or model-saving claim is inferred from a mock agent response.

## Licensing

Council source is MIT. Ejupi Labs generic icons retain their MIT notice. Company identity artwork
has separate provenance and is not relicensed by the icon package. See [asset provenance](third-party/brand-assets.md)
and the notices included with native preview packages.
