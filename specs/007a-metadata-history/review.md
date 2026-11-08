# Independent review: metadata history

**Status: review in progress; acceptance is pending.** This record summarizes
independent design and implementation reviews of feature 007a and the resulting
corrections. It is not a release or platform qualification statement.

## Findings and disposition

1. **Source provenance and privacy.** The original design could be read as
   guaranteeing that SQLite never physically reads excluded bytes, or as
   authenticating arbitrary copied databases. The specification now defines a
   logical application-level column-access boundary and explicitly trusts the
   Council-managed private store under its owner lock. The capture uses dedicated
   columns, bounded project-scoped queries and immutable snapshot rows; it does not
   decode opaque source records to fill the export. This does not authenticate a
   copied or same-user-tampered database, nor does it claim SQLite pages contain
   only eligible values.
2. **Native publication boundary.** Review identified unjournaled directory
   provisioning and unsafe staging/rename assumptions in early designs. The
   feature now uses the already-held private v2 root and a reserved history
   filename, a privately persisted native preview, held identity checks and
   anchored exclusive final-name creation after a durably acknowledged permit.
   Partial files are retained and reported as uncertain; a missing file is not
   proof of no publication. The private root is not asserted to be outside every
   possible Git repository. The native implementation and platform-specific
   tests still require complete qualification on the exact release source.
3. **Pre-permit failures and destination conflicts.** A target or parent change
   before the permit previously left an apparently waiting operation and hid the
   actionable cause. The operation is now frozen with a bounded reason and an
   explicit reconciliation action. Preview, create and permit transactions check
   unresolved work for the exact captured root and case-insensitive filename;
   this prevents a second publication intent from reusing a destination while a
   prior effect remains uncertain. A stranded prepared operation after owner
   restart can be frozen without asserting that its prior owner shut down cleanly.
4. **Owner acknowledgement and incident attribution.** History write faults
   initially produced global incidents without a durable link to the responsible
   history operation or request, so even a proven no-publication case could leave
   its own incident permanently fenced. The v8 schema now records immutable typed
   operation or request/fingerprint bindings when the owner records a history
   incident. A positive decision may resolve only its exact bound incident; other
   incidents, claims, in-flight effects and unrecorded owner causes remain fenced.
   The shared gate also checks unresolved history effects before opening.
5. **Reconciliation acknowledgement.** A lost acknowledgement of the
   reconciliation transaction itself could otherwise create a second unbound
   incident or be mistaken for a rollback. The owner now verifies the exact
   receipt, operation or incident revision and associated journal/decision under
   its history and control barriers. A committed receipt returns its recorded
   result; proven absence retains the relevant fence for the same request; an
   unprovable read retains an unrecorded in-memory fence and dirty owner epoch.
   The verification does not replay publication or release independent causes.
6. **UI continuation and pending intent.** Independent review checked that a
   CLI-originated operation is replayed using its original operation, request,
   preview, snapshot and destination, and that the browser saves and reads back
   an exact pending request before sending it. Storage failure blocks a new
   mutation, and delayed responses do not replace newer selections. This review
   does not substitute for the full rendered-browser acceptance suite.

## Evidence observed during review

Focused Store checks passed for v8 migration/schema handling, history export and
reconciliation, and the freeze/permit boundary. The latter covered both serial
orderings, concurrent Store contention, and a disposable missing-journal fixture
that retained its incident and global fence. Focused actual-owner history tests
were also reported passing during integration, including linked incidents and
reconciliation acknowledgement handling. These are partial results; record the
complete commands, platform and final source identity in `acceptance.md` only
after the required qualification has actually run.

## Remaining limitations and gates

- A committed or possibly committed publication permit prevents positive
  no-publication reconciliation. Recovery of a history-linked incident after a
  permit or an already published outcome is not implemented as an automatic
  release; such uncertainty remains fenced for inspection.
- Unknown, dirty or untracked owner history, an unrecorded in-memory fence, an
  incomplete journal, and unrelated incidents remain unresolved unless their
  own proof requirements are met. No generic incident is resolved by matching
  its text or timestamp.
- Windows can report directory sync as unavailable even after a successful file
  flush and verified output; this does not guarantee namespace survival across
  power loss. Unsupported or unverified native guarantees fail closed.
- This feature is incomplete metadata history. Content-bearing evidence export,
  backup, verified restore and reviewed cleanup remain separate mandatory work.
  No vendor credentials or real provider calls are needed for these tests.
- **Pending:** the complete local `npm run build`, `npm test` and `npm run check`
  qualification on final source; full rendered UI coverage; and exact-source
  hosted native and race CI. Passing an earlier or narrower run is not acceptance.
