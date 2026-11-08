# Project metadata history

Council can capture an immutable, project-scoped inventory of selected structured
metadata and export it locally. Every snapshot, records page and exported snapshot
states:

> Incomplete observational history. Not a backup, evidence export, restore, or proof of acceptance.

This feature is in the development preview and remains subject to its documented
acceptance and platform qualification. It does not complete the separate evidence
export, backup, verified restore or reviewed cleanup work.

## Capture and inspect

Open a project with Council, then use **Project history** in the local UI to
capture the selected project or inspect an existing snapshot ID. Switch to the
snapshot's project before inspecting it. The UI and CLI address the same saved
snapshot; inspection and export do not start an agent or model call.

For the CLI example below, replace the sample project ID with an ID returned by
`council projects open` and replace each request ID with a distinct, newly
generated 32-character lowercase hexadecimal ID. JSON responses contain the IDs
used by subsequent steps.

```text
council history capture --project 11111111111111111111111111111111 --request-id aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --json
```

Copy `data.snapshot.snapshotId` from the response. For example, if it is
`22222222222222222222222222222222`:

```text
council history show 22222222222222222222222222222222 --json
council history records 22222222222222222222222222222222 --kind tasks --limit 50 --json
council history records 22222222222222222222222222222222 --kind attempts --limit 50 --json
```

For another page, pass its returned `data.nextCursor` as `--cursor CURSOR` with
the same snapshot and kind. Cursors are bound to the immutable snapshot and
section. Counts and revisions are decimal strings; a recorded attempt phase is
not a task outcome or current process authority. A snapshot captures at most
2,048 tasks and 8,192 attempts and its encoded output is limited to 4 MiB;
Council reports an oversized capture instead of silently truncating it.

The inventory includes the selected project's eligible generated project, task
and attempt identities, relationships, revisions and recorded attempt phases.
It excludes task text, project paths, user files, artifact payloads, provider
output, vendor authentication, owner authority and legacy state. It does not
reconstruct dependency graphs, actual outcomes, checks, complete evidence or
provider usage. The application reads only selected structured database fields;
SQLite may still read physical pages containing other columns. Council does not
open vendor authentication files for this feature. The managed local store is a
trust boundary, not a cryptographic proof against same-user modification.

## Preview and export

Choose an unused filename in the reserved `history-*.json` namespace. The suffix
can contain ASCII letters, digits, underscores and hyphens, beginning with a
letter or digit. Council writes only in its existing private v2 state directory:
`<user-home>/.council/v2/`. It does not accept an arbitrary output path or import
an exported file. The preview shows the full destination and exact captured
snapshot before any output is created.

```text
council history export-preview --snapshot 22222222222222222222222222222222 --revision 1 --filename history-project.json --request-id bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb --json
```

Review `data.preview.destinationPath`, the snapshot ID, included categories and
omissions. Copy `data.preview.previewId` and its `revision`. For a preview ID of
`33333333333333333333333333333333` at revision `1`:

```text
council history export --preview 33333333333333333333333333333333 --revision 1 --request-id cccccccccccccccccccccccccccccccc --json
```

The output is private project metadata. Council does not stage or commit it or
include it in release packages; a user's home directory can itself be inside a
repository. The file contains the captured snapshot, selected records and a
SHA-256 integrity value for the generated metadata. The digest checks those
bytes; it does not make the inventory complete, verify original evidence or
authorize sharing. Existing targets and unsafe or changed destinations are
refused without overwrite. Do not assume a visible file is complete until the
operation reports `published`: an interrupted write can leave a partial file.

## Retry and inspect an interruption

Keep the exact request ID and arguments for every mutation. After a lost reply,
retry the same command with the same inputs; changing any input under that ID is
a conflict. Council returns the retained receipt and current operation state
without automatically repeating the publication. The UI's **Retry same request**
similarly sends the saved intent. A receipt confirms the recorded request, not
necessarily successful file publication.

Copy `data.operation.operationId` from the export response. For example:

```text
council history operation 44444444444444444444444444444444 --json
```

The response includes state, outcome, blockers, next action, destination and any
durability observation. `prepared` or `effect_started` is not proof of success;
`uncertain` remains fenced. Council retains a partial or unverified output and
does not automatically overwrite, delete or recreate it.

When the owner offers reconciliation, use the currently observed operation
revision and a fresh request ID:

```text
council history reconcile 44444444444444444444444444444444 --revision 2 --request-id dddddddddddddddddddddddddddddddd --json
```

This asks the owner to verify **no publication** from its own journal, owner
and worker observations. It does not ask Council to republish the file. A
positive decision requires the complete no-permit proof; a present or possibly
committed publication permit, an incomplete journal, undrained actor or dirty
owner history remains `unresolved`. The decision does not prove that the path
is absent or unchanged. Unrelated incidents and fences remain in force. If proof
is unavailable, retain the operation ID and its reported blockers for diagnosis.

On supported Windows filesystems, a `published` operation can report a successful
file flush with `directorySync: "unavailable"`. This is an explicit limitation,
not a guarantee that the directory entry survives power loss. Other unsupported
filesystem guarantees or uncertain writes remain unsupported or uncertain; no
universal power-loss or network-filesystem durability is promised.
