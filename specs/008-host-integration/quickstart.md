# Quickstart and Acceptance: MCP Host Integration

These scenarios were written at design time; the `council mcp` command and tools now exist, and executed evidence and limits are recorded in [acceptance.md](acceptance.md). This document is not itself acceptance evidence. Use disposable Council state, a real offline MCP client and fake agents; do not open vendor credentials or issue live provider calls.

1. Start `council mcp` through a real stdio client, negotiate an explicitly supported protocol version, list the documented closed catalog and verify every stdout frame is protocol JSON while diagnostics go only to stderr. Exercise malformed, duplicate-key, oversized, unknown and unsupported requests without effects.
2. With no owner, connect and observe one background owner start. Open a disposable project and start a fake task from MCP, then inspect it through CLI and UI. Reverse the direction with CLI- and UI-created work. Start a second host concurrently and verify the same task/owner IDs and shared capacity.
3. Commit a task or graph mutation, lose the response, reconnect and retry its exact durable request ID plus complete original authority through the host and another interface. Check the same receipt and one effect; effect-changing input must conflict without silently refreshing any authority.
4. Wait on an active task and cancel or disconnect the host. Verify the attempt remains active and inspectable; then use explicit Stop and observe accepted cancellation, cleanup and terminal evidence through another interface.
5. Reconstruct a long binary and multibyte artifact through bounded base64 pages with a frozen end boundary and compare exact bytes and digest. Verify errors do not return partial evidence as complete or cause a rerun.
6. Exercise accepted routing, workflows, graph controls, pause/checkpoint/resume, independent incidents and incomplete history through the catalog; compare owner IDs, captured revisions, outcomes and limits with UI/CLI. Check old compatible maintenance Stop without replacing an incompatible owner.
7. Verify clean local setup in paths with spaces and Unicode. Run independent review, `npm run build`, `npm test`, `npm run check`, actual protocol client interoperability and six native plus race CI on the exact source. Document unavailable versions, skipped tests, dependencies/notices and no live-provider qualification.

Full evidence export, production backup/restore, reviewed cleanup and remaining harness integrations remain mandatory before release.
