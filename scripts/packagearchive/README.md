# Council workspace preview

Version 2.0.0-dev. This native preview opens local Git projects, saves an operating
preference, observes five harness installations and prepares versioned task drafts.
Task execution is unavailable in this preview. Installation observations do not
qualify authentication, models, account capacity or a working agent adapter.

Extract every file, then run `./council --version` and `./council ui` from a terminal.
On Windows, use `council.exe` or `.\council.exe`. `ui --no-open` reports the private
local URL without opening a browser. Keep that URL private; it grants access to the
running local owner. The foreground owner keeps running until Ctrl+C, including
when its browser tab closes. `--help` lists the supported commands.

Council needs no Node.js, npm or Go installation. Git must be installed and available
on PATH for project operations. The UI and SQLite 3.54.1 are embedded in the binary;
the UI loads no external assets and startup makes no model calls. Use the archive
matching your operating system and processor. Platform qualification is recorded
separately from cross-compilation in the source repository's acceptance evidence.

Private application data is stored under your user home's `.council/v2/` directory.
This preview does not open or migrate legacy Council state. The requested autonomous
policy is a saved preference; this preview neither runs tasks nor grants harness
permissions. Vendor credentials remain with their vendors.

The adjacent `.zip.sha256` file checks the complete archive. `SHA256SUMS.txt` checks
every distributed file except itself. Keep the included LICENSE and third-party
notices with the binary. Ejupi Labs company artwork remains authorized company
identity; generic interface icons carry their separate MIT notice.

Source and acceptance evidence: https://github.com/Ejupi-Labs/council/tree/refactor
