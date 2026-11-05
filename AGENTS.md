# Council development

## Clean rebuild on `refactor`

The user has requested a replacement built from functional requirements, rather than an
incremental port of the legacy runtime. Work on the exact branch `refactor`; do not merge the
replacement into main without a further user request. The functional reference is commit
`27822baa3aee663366a870bde7fb1190649d3418`.

- Read `docs/refactor/vision.md`, `docs/refactor/functional-requirements.md` and
  `.specify/memory/constitution.md` before designing replacement features.
- All newly authored requirements, specifications, product copy and engineering documents are
  English. Keep original evidence verbatim when fidelity requires it.
- Use Specify for one feature at a time: specification, design/plan, tasks, implementation,
  independent review, corrections and acceptance evidence before implementing the next feature.
- Keep Spec Kit feature context separate from the Git branch. Do not install automatic Git
  branching hooks. Parallel work within the current feature needs explicit file ownership.
- Technology is open, including Rust. Choose and record the replacement stack in the first
  feature's technical plan. The legacy Node/SQLite implementation is reference material, not
  a language or storage constraint on the replacement.
- The new product prioritizes suitable small models, existing account capacity, focused context,
  minimal configuration and a persisted autonomous full-access preference. Report unsupported
  harness capabilities and unknown telemetry honestly.
- Unlike the legacy product's blanket no-merge behavior below, the replacement may integrate a
  verified local change automatically when the saved task policy authorizes that destination.
  Preserve user edits; external publication must be explicitly included in task scope.
- Use the verified Ejupi Labs assets and notices in `docs/refactor/design-identity.md`.
- Never inspect or copy vendor credentials. Development tests use fake agents; live provider
  qualification requires a specific user-requested live task.
- Preserve local data under `.council/`, outside Git and release artifacts. Do not silently open,
  migrate or delete legacy collaboration state while implementing the replacement.

The selected replacement uses Go 1.27.3 and an embedded authored TypeScript UI. Runtime state is
`<user-home>/.council/v2/`; legacy reference files under `reference/legacy/` are excluded from build
and release inputs. Run `npm run build`, `npm test` and `npm run check`; commit compiled browser
assets under `internal/webui/dist/` with source changes. Native binaries are ignored build outputs.
Use native Go tooling where the current semantic server cannot serve Go; generic Serena source
edits remain available. Runtime npm dependencies remain prohibited.

The requirements below still govern edits to the legacy implementation while it is present.
Replacement feature plans must provide meaningful equivalent verification and release rules;
keep the `npm test` and `npm run check` developer commands working if the tooling changes.

## Legacy implementation

Council is a local TypeScript MCP server and CLI. Node 24.18.1+ with SQLite 3.51.3+ is required for patched built-in SQLite.

- Keep subprocess invocations shell-free and Windows compatible. Do not inspect or copy vendor credentials.
- MCP stdout is protocol-only. Send diagnostics to stderr.
- Workflow results must distinguish completed, failed and cancelled work; partial results never imply success.
- Native Codex read mode uses its read-only sandbox; Claude uses a read/search allowlist. ACP read mode denies new tool approval requests and scopes its file service to project files. No universal OS sandbox is promised. Write workflows use isolated Git worktrees and never merge automatically.
- Persist collaboration state under `.council/`; keep it out of Git and npm packages.
- Configure SQLite writes with writeStatement() to preserve exact 64-bit result metadata without changing query types. Reject unsafe public numeric cursors before commit.
- Run `npm test` and `npm run check` before handing off changes. Tests use local fake agents; live agent calls require a user-requested task.
- Reference implementations are inspiration. Retain attribution if code is ever copied.
- Runtime code has no third-party npm dependencies. The official MCP SDK is development-only interoperability tooling.
- Commit built dist/ and schemas/ with source changes so direct plugin checkouts have working entrypoints. Run npm run build before staging them.
