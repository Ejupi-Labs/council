package cli

import (
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Every caller receives its own existing fixture home, owner and cleanup before assertions.
func workflowCLIFixture(t *testing.T) *cliFixture {
	t.Helper()
	f := newCLIFixture(t)
	if e := os.MkdirAll(filepath.Join(f.repo, "docs"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(f.repo, "docs", "note.md"), []byte("CONTROLLED LOCAL WORKFLOW TEST DATA\r\nold\r\n東京\r\n"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"add", "docs/note.md"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "controlled workflow document"}} {
		c := exec.Command("git", append([]string{"-C", f.repo}, args...)...)
		if out, e := c.CombinedOutput(); e != nil {
			t.Fatalf("controlled workflow Git setup failed: %v %s", e, out)
		}
	}
	return f
}

func workflowCLIMap(t *testing.T, e domain.Envelope) map[string]any {
	t.Helper()
	raw, err := json.Marshal(e.Data)
	var data map[string]any
	if err != nil || json.Unmarshal(raw, &data) != nil {
		t.Fatal("workflow CLI data is not an object")
	}
	return data
}

func awaitWorkflowCLI(t *testing.T, f *cliFixture, task string, predicate func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		data := workflowCLIMap(t, f.run("tasks", "workflow", task))
		if predicate(data) {
			return data
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("controlled workflow did not reach expected state")
	return nil
}
