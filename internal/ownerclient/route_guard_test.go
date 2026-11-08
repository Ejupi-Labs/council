package ownerclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Route validation happens before state access and before any owner request.
// Body validation is a separate, mandatory MCP gate.
func TestRouteGuardRejectsPathsAndQueriesBeforeAttachment(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	root := filepath.Join(t.TempDir(), "not-created")
	client, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	tests := []Request{
		{Operation: "graphs.nodes.stop", PathIDs: []string{id, "../node"}, RequestID: id, Body: []byte(`{"requestId":"` + id + `"}`)},
		{Operation: "tasks.get", PathIDs: []string{id}, Query: map[string]string{"arbitrary": "1"}},
		{Operation: "tasks.get", PathIDs: []string{id}, Query: map[string]string{"limit": ""}},
		{Operation: "tasks.start", PathIDs: []string{id}, RequestID: id, Body: []byte(`{"requestId":"` + id + `"}`)},
		{Operation: "tasks.get", PathIDs: []string{id, id}},
	}
	for _, req := range tests {
		if result := client.Call(context.Background(), req); result.Envelope.Error == nil {
			t.Fatalf("invalid route was accepted: %s", req.Operation)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("invalid route touched owner state: %v", err)
		}
	}
}
