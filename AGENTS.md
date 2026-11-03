# Council development

Council is a local TypeScript MCP server and CLI. Node 24+ is required for built-in SQLite.

- Keep subprocess invocations shell-free and Windows compatible. Do not inspect or copy vendor credentials.
- MCP stdout is protocol-only. Send diagnostics to stderr.
- Workflow results must distinguish completed, failed and cancelled work; partial results never imply success.
- Native Codex read mode uses its read-only sandbox; Claude uses a read/search allowlist. ACP read mode denies new tool approval requests and scopes its file service to project files. No universal OS sandbox is promised. Write workflows use isolated Git worktrees and never merge automatically.
- Persist collaboration state under `.council/`; keep it out of Git and npm packages.
- Run `npm test` and `npm run check` before handing off changes. Tests use local fake agents; live agent calls require a user-requested task.
- Reference implementations are inspiration. Retain attribution if code is ever copied.
- Runtime code has no third-party npm dependencies. The official MCP SDK is development-only interoperability tooling.
- Commit built dist/ and schemas/ with source changes so direct plugin checkouts have working entrypoints. Run npm run build before staging them.
