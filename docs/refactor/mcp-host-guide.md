# Connect an MCP host

Feature 008 is under development and has not completed its acceptance gate.
The commands below describe the current local interface; consult its acceptance
record before relying on it for production work.

Council exposes a local stdio MCP server using the same persistent owner as the
UI and CLI. Run the native executable directly with the single argument `mcp`.
The host must use pipes for stdin and stdout; Council sends diagnostics only to
stderr. The server does not require a separate URL, bearer token, account or
model setting. It supports MCP protocol versions `2025-06-18` and `2025-11-25`.

For an MCP host that accepts a `command` and `args` configuration, substitute
the absolute path of the actual executable. For example, a common JSON shape is:

```json
{
  "mcpServers": {
    "council": {
      "command": "/absolute/path/to/council",
      "args": ["mcp"]
    }
  }
}
```

On Windows the JSON command can be `C:\\path with spaces\\council.exe`.
Enter it as the executable path, without surrounding shell quotes or an
intervening shell. Host configuration locations and support vary; the
automated tests currently use the official Go MCP SDK client, not a claim that
any particular editor or provider host has been qualified.

| Host or protocol | Current evidence |
| --- | --- |
| MCP stdio `2025-06-18`, `2025-11-25` | Offline SDK and raw-protocol fixtures; full feature acceptance is pending. |
| Default Go MCP SDK 1.8.0 client | Offline fallback and stdio setup tested. |
| Specific editor/provider host | Not yet qualified; check that host's own setup documentation. |

Use `council_owner_meta` to inspect the continuing owner's metadata and
`council_agents_list` to inspect discovered connections. The project, draft,
task and graph tools share identities and records with the UI and CLI. Read
the result's `status` and `envelope`; admission or a successful wait does not
by itself mean the task completed or its changes were accepted.

Every mutation takes its complete owner request under `input`, including a
caller-chosen 32-character lowercase hexadecimal `requestId`. Keep the complete
request and exact captured authority for retries. A new JSON-RPC request ID is
only a transport correlation ID and does not replace the durable Council ID.
If a reply is lost, retry the same intent and request ID or inspect the existing
record; a lost reply does not prove that the operation was rolled back.

For `council_artifacts_bytes`, capture the first page's `through` value, pass it
unchanged with subsequent offsets, and concatenate the decoded `bytesBase64`
pages. `eof` refers to the selected boundary; check `streamClosed` and the
available integrity information separately. Cancelling a wait or disconnecting
the host stops observation, not the owner's task. Stop, pause, resume and
reconciliation are separate explicit operations.

The history tools expose **incomplete metadata history** only. They do not
provide a backup, restore or complete evidence export. Existing user data stays
in `<user-home>/.council/v2/`; this server does not read vendor credentials or
silently migrate legacy collaboration state.
