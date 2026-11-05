# Foundation acceptance quickstart

Planned procedure, not passed evidence. Use disposable Git projects, an isolated user home and
fake installation metadata. Never use vendor credentials or invoke installed agents.

## Build and launch

Development: Git, Go auto-toolchain support, Node24.18.1+ and npm. Run `npm ci`, `npm run build`,
`npm test` and `npm run check`. Build reports the native binary location. A release user needs
only the native binary, Git and browser, without development packages or compilers.

Run `council --version --json`, then `council ui --no-open --json`. Open the emitted private URL;
omit its fragment from logs/screenshots. Browser closure leaves owner alive. A second UI command
reuses it. Ctrl+C shuts down; it does not fabricate task cancellation.

## Project and draft

1. Create a Git fixture in a Unicode/spaces path with staged and unstaged edits. Record source
   bytes, index, branch and HEAD. Open using UI and `council projects open PATH --json`.
2. Open a physical alias: same ID. Distinct folder: different ID. A selected subdirectory stays
   selected while repository root is shown separately. Invalid/replaced paths reject honestly.
3. Observe requested Autonomous full access; save once using current policy revision, restart
   and confirm. No effective harness permission is claimed.
4. Save a multiline Unicode Auto draft, get it through the other interface, compare exact UTF-8
   including BOM, U+FFFD, whitespace and CRLF. Status stays Draft / Not run.
5. Change policy; prior snapshot remains. Explicit update with --use-current-policy changes the
   new snapshot; historical revision retains full old text/policy. Restart and cross-read again.
6. Recheck source/index/branch bytes unchanged after every preparation operation.

## Discovery, authority and faults

Run agents list/refresh against fixture PATH/native metadata. Exactly five ordered entries;
installed differs from unknown auth/model/allowance/occupancy. Shell-only wrapper unsupported,
missing candidate missing, metadata errors unavailable. Invocation/credential sentinels untouched.
Refresh cannot clear unsaved text or change project selection.

Same mutation ID/input replays exact original status/data/receipt. Changed input conflicts. Stale
revision preserves text. Inject known rollback and uncertain commit: neither says Saved; retry
same identity reconciles. Revision/sequence overflow rejects before commit.

Exercise unauthorized private GET, wrong Host, foreign/null Origin, duplicate/unknown query/JSON,
invalid UTF-8/lone surrogates, 4MiB text and 32MiB body. Errors are bounded and omit private text.
Use actual subprocesses to verify native owner exclusion/recovery and DACL or Unix modes.

## UI and release

Inspect real desktop and 360px renders against approved Ejupi Labs assets/tokens. Complete main
flow by keyboard, labelled controls, focus, readable feedback and reduced motion. Delay responses
while typing/switching projects: newer input/selection survives; conflicts retain local edits.

Extract allowlisted native preview into a disposable path. Run from another directory with no
Node/Go/node_modules available, exercise version/startup/CLI. Inspect archive for state, tokens,
credentials, task text and unintended machine paths. Record actual OS/architecture, runtime
SQLite version, checksums, commands and limits in acceptance.md. Cross-compilation alone is not
runtime qualification. No working adapter, delivered task or measured model-saving claim in001.
