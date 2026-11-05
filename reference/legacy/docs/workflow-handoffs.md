# Reliable workflow handoffs

Use this guide when a host coordinates implementation, review and later integration. Council retains agent results and isolated patches. The host remains responsible for current phase instructions, project prerequisites and acceptance of the resulting change.

For example: phase 2 changes a Java class, an independent reviewer inspects the changed source, and the host runs the required build. A completed author result establishes only that the author call finished. It does not establish that review, the build or integration succeeded.

## Prepare the current phase

- Identify the current task, acceptance criteria, relevant source and accepted base commit. Remove obsolete phase restrictions from supplied context. Council cannot infer which contradictory instruction the coordinator intended.
- Inspect the plan before dispatch. Reviewers remain read-only inside a write workflow. Match their requested checks to their actual tools, as shown below.
- Check required project metadata, dependencies and runtimes. Write worktrees start from committed HEAD. Original-checkout edits, ignored metadata and local environment changes are not copied automatically.
- If ignored metadata is necessary, prepare an explicit non-secret allowlist in the intended worktree. Keep vendor credentials, environment secrets and unrelated ignored trees out of this preparation. Do not add automatic copying or arbitrary initialization hooks.
- Choose call timeouts for the actual task and backend. Split work at meaningful acceptance boundaries when useful. Run independent tasks in parallel only when their prerequisites and ownership permit it. A sequential author/review/revision workflow is not parallel implementation.

| Reviewer transport | Available inspection path | Checks to leave to the host |
| --- | --- | --- |
| Native Codex | Commands within the configured read-only sandbox | Commands the sandbox cannot start or permit; writes |
| Native Claude | Read, Glob and Grep | Shell commands, builds, tests and tools outside that allowlist |
| Council direct ACP | Scoped text-file reads through the client's file service | Terminal commands, writes and operations needing a new tool approval |

A task-level shell ban can prevent native Codex from reading source when its read path uses commands. Prefer a compatible reviewer or provide the complete relevant source explicitly. Do not relax read permissions to make a review appear successful. Ask the reviewer to identify inspected files or supplied evidence, actual checks and remaining gaps. A response that says no source was inspected is not source-review approval, even when the agent process completed.

Each dispatched prompt identifies the actual step directory, including the author's isolated worktree during review. Resolve project-relative source there. Paths in the original task or a peer's report do not extend ACP or native Antigravity workspace access. If required evidence is outside that boundary, have the host supply it within the authorized task or record the blocked check. A denied outside-project read is not a reason to turn on broad approvals.

Junie 26.7.20 can complete prose source reviews but may emit inspection commentary before a room ballot even when the whole-turn contract forbids it. ACP provides no distinguishing final-answer metadata in the reproduced case. Council keeps such rooms at `needs_attention` and does not execute their tasks. Use an explicitly chosen compatible voting participant, or use Junie for prose review/advice; Council does not silently substitute one. See [the reproduction](junie-acp-ballot-reproduction.md).

## Use one verified Windows runtime

Choose the full path to a maintained Node installation and use it for Council. `doctor` reports `runtime.node.executable`, the Node version, embedded SQLite version and separate compatibility results. It does not open databases, recover runs or call models. Runtimes that can load Council but fail its version requirements exit with code 1 after reporting. Older Node versions without `node:sqlite` cannot load the CLI.

```powershell
$councilNode = 'C:\path\to\supported-node\node.exe'
$councilCli = 'C:\path\to\council\dist\cli.js'
$projectRoot = 'C:\path\to\project'
& $councilNode $councilCli doctor --workspace $projectRoot
if ($LASTEXITCODE -ne 0) { throw 'Resolve runtime readiness before opening Council state.' }
& $councilNode $councilCli connection --workspace $projectRoot
```

The generated connection uses that absolute Node executable. Updating PATH alone may not affect an `npm.cmd` launcher that selects a sibling `node.exe`. For development commands, invoke the selected Node with the intended `npm-cli.js` path, or use a verified installation whose Node and npm agree. The existing SQLite guard refuses unpatched runtimes before database access.

Independent native Codex children discard only the inherited `CODEX_WINDOWS_REGISTERED_CORE` and `CODEX_WINDOWS_SANDBOX_PACKAGE_FAMILY` Desktop dispatch markers, including case variants. These markers can misroute sandbox command startup from a standalone CLI. Council preserves user/account configuration and its explicit `--sandbox read-only` or `workspace-write` arguments. This is not a sandbox bypass or a claim that native command startup works on every installation. See the official [Windows sandbox troubleshooting guide](https://learn.chatgpt.com/docs/windows/windows-sandbox) for installation and managed-policy failures.

## Preserve evidence through a handoff

Keep the complete structured result of a launch tool. If it returns a process-session handle, retain that handle along with the Council run ID. A tool's partial output alone may omit how to resume waiting. For MCP, retain the run ID and wait server-side; waiting does not dispatch another model call. For CLI, the owner process must remain alive until its final result.

Use task and context files to avoid shell interpolation. Save them as UTF-8. Council accepts an initial UTF-8 BOM and rejects malformed bytes instead of replacing them. JSON input also rejects duplicate keys, including escaped spellings of the same key. Decode, parse and applicable schema checks run before the CLI opens collaboration state. Configuration errors and state-dependent checks still have their own validation paths.

The following CLI example preserves the final JSON, stderr launch receipt and exit code. Create the destination directory and input files first. Choose distinct filenames for each attempt so previous evidence remains available.

```powershell
$receiptRoot = 'C:\path\to\existing-receipts'
& $councilNode $councilCli run --workspace $projectRoot `
  --mode review --agents claude,codex --write `
  --task-file 'phase-02-task.txt' --context-file 'phase-02-context.txt' `
  1> (Join-Path $receiptRoot 'phase-02-result.json') `
  2> (Join-Path $receiptRoot 'phase-02-events.log')
$councilExit = $LASTEXITCODE
Set-Content -LiteralPath (Join-Path $receiptRoot 'phase-02-exit.txt') -Value $councilExit -Encoding utf8
```

The example assumes configured `claude` and `codex` aliases, sufficient task-specific timeouts and a committed Git base. Use `council plan` with the same workflow arguments first. PowerShell output redirection behavior varies by version; use a current PowerShell that preserves native UTF-8 stdout when consuming Unicode JSON. Inspect the retained Council result directly if the host's capture altered the file.

## Settle work honestly

Read the terminal result and the individual steps. Keep failed, timed-out and cancelled work distinct from completed work. An author's partial artifact does not imply that reviewers or revision ran. Retain partial evidence and inspect the managed worktree before any new attempt. Do not automatically replay writes or relabel a failed run after independent host verification.

If the coordinator maintains a separate phase plan, update it from actual results. Distinguish author completion, review findings, revision, host verification, integrated change and accepted commit as applicable. Council does not update arbitrary external plan files or merge retained worktrees automatically.

When an adapter fails, retain the run ID, step, backend, actual runtime report, terminal status and safe error summary. ACP read denials may include a canonical agent-reported tool kind; the label does not authorize the operation. Antigravity invalid-step errors include field types and allowlisted protocol labels, excluding raw text, paths, commands and unknown values. Do not reconstruct a missing historical frame from a later successful call. A provider/channel fix needs evidence from the failing protocol event and separate qualification.
