# Council

A local workspace for orchestrating coding agents across harnesses and providers.

This is the **refactor** development branch. The clean rebuild is being delivered one specified
feature at a time. The accepted functional reference is preserved under [reference/legacy](reference/README.md);
main remains unchanged.

## Current preview

The workspace foundation opens existing Git projects, remembers the requested autonomy preference,
inspects five harness installations and saves complete versioned task drafts. The embedded English
UI uses approved Ejupi Labs artwork and icons. UI and CLI share the same private local records.

**Task execution is not available in this foundation preview.** Installation detection does not
verify authentication, models, subscription allowance or working agent adapters. Automatic routing,
managed execution, review/integration and MCP arrive through the ordered feature plan.

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
```

On Windows supply a native absolute path, quoting spaces normally. `council ui --no-open --json`
reports a private local launch URL instead of opening a browser. Keep that URL private. Closing the
browser leaves the foreground owner running; Ctrl+C shuts it down. Other CLI commands contact that
owner, or hold exclusive transient ownership when it is absent.

New state lives at `<user-home>/.council/v2/`, separate from legacy data. Council does not inspect
vendor credentials or run vendor executables during installation discovery. Autonomous full access
is the requested default; effective permissions remain unverified until supported execution exists.
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

Each feature follows Specify specification → design → tasks → implementation → independent review
→ corrections and acceptance before the next begins. Technology and limitations are documented in
the active feature's design. No release or model-saving claim is inferred from a mock agent response.

## Licensing

Council source is MIT. Ejupi Labs generic icons retain their MIT notice. Company identity artwork
has separate provenance and is not relicensed by the icon package. See [asset provenance](third-party/brand-assets.md)
and the notices included with native preview packages.
