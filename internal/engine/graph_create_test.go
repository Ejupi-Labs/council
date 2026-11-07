package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/project"
)

func TestGraphCreationFactsReadExactProjectWithoutChangingWorkingOrIndex(t *testing.T) {
	e, id, repo := testEngine(t)
	working := filepath.Join(repo, "graph-prerequisite-dirty.txt")
	content := []byte("\xef\xbb\xbfuntouched dirty 東京\r\n")
	if err := os.WriteFile(working, content, 0600); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := e.captureGraphCreateFacts(context.Background(), id)
	if err != nil || facts.ProjectIdentity == "" || facts.RepositoryIdentity == "" || len(facts.BaseCommit) != 40 || facts.Branch == nil || !strings.HasPrefix(*facts.Branch, "refs/heads/") {
		t.Fatal("Incomplete actual project creation facts", facts, err)
	}
	if err := project.ValidateIdentity(repo, facts.ProjectIdentity); err != nil {
		t.Fatal("Facts did not bind the selected actual directory", err)
	}
	after, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(index, after) {
		t.Fatal("Read-only graph prerequisites changed the user index", err)
	}
	actual, err := os.ReadFile(working)
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatal("Read-only graph prerequisites changed user working bytes", err)
	}
}
