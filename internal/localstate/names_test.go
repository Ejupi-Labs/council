package localstate

import (
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryOutputNamespaceExcludesOwnerControlFiles(t *testing.T) {
	for _, name := range []string{ownerLockName, ownerDescriptorName, ownerTemporaryPrefix + "abc.tmp", databaseName, databaseName + "-wal", databaseName + "-shm", databaseName + "-journal"} {
		if strings.HasPrefix(strings.ToLower(name), HistoryOutputPrefix) || domain.ValidHistoryFilename(name) {
			t.Fatalf("Council control name %q conflicts with history output namespace", name)
		}
	}
	for _, name := range []string{"history-report.json", "history-2026_11-08.json"} {
		if !strings.HasPrefix(name, HistoryOutputPrefix) || !domain.ValidHistoryFilename(name) {
			t.Fatalf("valid history output %q does not use reserved namespace", name)
		}
	}
}
