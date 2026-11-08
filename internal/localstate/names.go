package localstate

// HistoryOutputPrefix reserves history-*.json in the owner-locked v2 root for
// explicitly requested metadata exports. Council control and temporary files
// must never use this namespace; exporters still require strict basename checks
// and exclusive creation.
const HistoryOutputPrefix = "history-"

const (
	ownerLockName        = "owner.lock"
	ownerDescriptorName  = "owner.json"
	ownerTemporaryPrefix = "owner-"
	databaseName         = "workspace.sqlite3"
)
