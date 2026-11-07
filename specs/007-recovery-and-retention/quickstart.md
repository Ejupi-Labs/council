# Quickstart: Verify Pause and Recovery

This is a validation guide for implementation; no result below is asserted as passed. Use a disposable local project and controlled fake agents only. Do not inspect vendor credentials or legacy state, and do not run live provider tasks as part of this feature.

## Prerequisites

- Check out the reviewed feature source on physical `refactor`; record the exact revision.
- Install repository-pinned Go and Node development tools and browser dependencies. Use a temporary isolated home/state root and a disposable Git project containing known dirty work; retain a separate comparison of working files and index.
- Use the exact CLI syntax and owner operations in [pause-reconciliation.md](contracts/pause-reconciliation.md); do not treat a setup failure as a completed scenario.

## Required local commands

Run serially from the repository root after implementation:

```powershell
npm run build
npm test
npm run check
```

Record terminal exit codes, source revision and log identities. Build refreshes committed `internal/webui/dist/`; verify generated assets match source and include changed assets in the reviewed commit. Focused Go/browser tests support development but do not replace full commands.

## Controlled journeys

1. Start a fake graph with a retained completed member, a running member and waiting dependent. Pause during execution; assert zero admissions after the accepted boundary, the admitted work settles, and the checkpoint shows pausing/paused honestly. Restart the owner, inspect retained identities, explicitly resume, and verify no completed call or effect repeats. Repeat for standalone and graph-member scopes, overlapping group/member pauses and an independent sibling.
2. Race pause with admission, first GO, a completion callback, guidance, Stop, budget expiry and a controlled owner loss. Verify authority is checked at the serialized boundary and every accepted Stop/failure keeps precedence. Inject checkpoint persistence failure and verify no unsafe admission or falsely durable paused state.
3. Crash a controlled owner after partial output and retain its evidence; verify no replacement starts automatically. Separately exercise actual still-held exact owned process-tree cleanup proof with complete effects/evidence and confirm reconciliation releases only its claim; keep a second blocker active and verify it remains. Repeat with missing child proof or missing evidence and verify the incident remains fenced. Do not use PID assertions as cleanup proof.
4. Initiate a pause or reconciliation in UI or CLI, observe it from the other and retry the exact request after a lost acknowledgement. Confirm matching identities, no duplicate dispatch, retained drafts and no newer selection overwritten by a stale response. Check keyboard access, visible focus, named controls, text status and desktop/narrow layouts in actual browser states; all polling and inspection create zero model calls.

## Review and release evidence

Independently review source, concurrency/failure paths, schema migration, physical process evidence and browser states. Correct blockers and rerun affected checks. Map every scoped requirement and success criterion to actual offline evidence; record exact terminal local and six-native-plus-Linux-race hosted results separately from unperformed live provider qualification. Commit acceptance before beginning the immediately following scoped export/backup/restore/cleanup feature. Its unresolved arbitrary-content credential-provenance problem must remain visible; no retention capability is implied by this feature's acceptance.
