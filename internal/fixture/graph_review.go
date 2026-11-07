package fixture

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// GraphReviewArgsWithContext selects the fixed offline graph reviewer. It is
// never a public author scenario, user script or installed vendor command.
func GraphReviewArgsWithContext(harness, response, expectedDirectory, workspaceDirectory string) ([]string, error) {
	args, err := GraphArgsWithContext(harness, "graph-response", "author", response, expectedDirectory)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(workspaceDirectory) || filepath.Clean(workspaceDirectory) != workspaceDirectory {
		return nil, errors.New("Select the exact absolute controlled workspace directory.")
	}
	args[4], args[6] = "graph-review", "review"
	return append(args, "--workspace-directory", workspaceDirectory), nil
}

func runGraphReview(harness, response, directory, workspaceDirectory string) int {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, domain.MaxPromptBytes+1))
	if err != nil || len(input) > domain.MaxPromptBytes {
		return 2
	}
	reads, err := readGraphFixtureContext(input, directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, "The controlled graph reviewer requires complete exact context access.")
		return 9
	}
	line := bytes.IndexByte(input, '\n')
	var prompt workflowPrompt
	if line < 0 || json.Unmarshal(input[line+1:], &prompt) != nil || prompt.Role != "review" || !domain.ValidID(prompt.Subject.ID) || !domain.ValidSHA256(prompt.Subject.SHA256) || !domain.ValidSHA256(prompt.ConditionsSHA256) {
		return 2
	}
	report, err := graphReviewReport(prompt, workspaceDirectory)
	if err != nil {
		fmt.Fprintln(os.Stderr, "The controlled graph reviewer could not verify the supplied physical subject.")
		return 9
	}
	inspections, err := graphFinalFixtureInspections(input)
	if err != nil {
		return 9
	}
	report.InspectedEvidence = append(report.InspectedEvidence, inspections...)
	body, err := json.Marshal(report)
	if err != nil {
		return 3
	}
	observations := map[string]any{"fixture_input_sha256": contextRefSHA(input), "fixture_context_reads": reads}
	if harness == "claude-code" {
		observations["subtype"], observations["session_id"] = "init", "council-graph-review-fixture"
		if emitClaude("system", observations) != nil || emitClaude("result", map[string]any{"subtype": "success", "is_error": false, "session_id": "council-graph-review-fixture", "result": string(body), "structured_output": report}) != nil {
			return 3
		}
		return 0
	}
	if writeResponse(response, string(body)) != nil || emit("thread.started", map[string]any{"thread_id": "council-graph-review-fixture"}) != nil || emit("turn.started", observations) != nil ||
		emit("item.completed", map[string]any{"item": map[string]any{"id": "graph-review", "type": "agent_message", "text": string(body)}}) != nil || emit("turn.completed", nil) != nil {
		return 3
	}
	return 0
}

// This deterministic fake checks the actual fixed graph result and reads all
// ordinary files in the supplied candidate. It does not attest live model quality.
func graphReviewReport(prompt workflowPrompt, workspaceDirectory string) (domain.ReviewReport, error) {
	report := domain.ReviewReport{SchemaVersion: domain.ReviewContract, Subject: prompt.Subject, ConditionsSHA256: prompt.ConditionsSHA256, Outcome: "approved",
		InspectedEvidence: []domain.ReviewInspection{}, Findings: []domain.ReviewFinding{}, Gaps: []domain.ReviewGap{}, ResolvedFindingIDs: []string{}}
	invalid := errors.New("The controlled graph review subject is unavailable.")
	if len(prompt.Evidence) < 1 || len(prompt.Evidence) > 64 {
		return report, invalid
	}
	var selected []byte
	seen := map[string]bool{}
	for _, reference := range prompt.Evidence {
		if reference.Ref == "" || seen[reference.Ref] || !filepath.IsAbs(reference.Path) {
			return report, invalid
		}
		seen[reference.Ref] = true
		file, err := os.Open(reference.Path)
		if err != nil {
			return report, err
		}
		stat, statErr := file.Stat()
		body, readErr := io.ReadAll(io.LimitReader(file, workspace.MaxFileBytes+1))
		closeErr := file.Close()
		if statErr != nil || readErr != nil || closeErr != nil || !stat.Mode().IsRegular() || len(body) > int(workspace.MaxFileBytes) {
			return report, invalid
		}
		report.InspectedEvidence = append(report.InspectedEvidence, domain.ReviewInspection{Ref: reference.Ref, Scope: reference.Scope, Paths: append([]string{}, reference.Paths...)})
		if prompt.Subject.Kind == "candidate" && reference.Ref == "selected-candidate-manifest" && reference.Scope == "whole_candidate" ||
			prompt.Subject.Kind == "response" && reference.Ref == "selected-response" && reference.Scope == "whole_response" {
			if contextRefSHA(body) != prompt.Subject.SHA256 || len(reference.Paths) != 0 {
				return report, invalid
			}
			selected = body
		}
	}
	if selected == nil {
		return report, invalid
	}
	if prompt.Subject.Kind == "response" {
		if string(selected) != GraphResponse {
			return report, invalid
		}
		return report, nil
	}
	if prompt.Subject.Kind != "candidate" {
		return report, invalid
	}
	var manifest workspace.Manifest
	if json.Unmarshal(selected, &manifest, json.RejectUnknownMembers(true)) != nil || len(manifest.Entries) > workspace.MaxEntries {
		return report, invalid
	}
	workspaceDirectory, err := filepath.EvalSymlinks(workspaceDirectory)
	if err != nil {
		return report, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return report, err
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return report, err
	}
	selectedDirectory, err := filepath.Rel(workspaceDirectory, cwd)
	if err != nil || !filepath.IsLocal(selectedDirectory) {
		return report, invalid
	}
	root, err := os.OpenRoot(workspaceDirectory)
	if err != nil {
		return report, err
	}
	defer root.Close()
	fixedA := filepath.ToSlash(filepath.Join(selectedDirectory, GraphFileA))
	fixedB := filepath.ToSlash(filepath.Join(selectedDirectory, GraphFileB))
	fixedJoined := filepath.ToSlash(filepath.Join(selectedDirectory, GraphJoinedFile))
	fixedConflict := filepath.ToSlash(filepath.Join(selectedDirectory, GraphConflictFile))
	var total int64
	observedGraphResult := false
	paths := map[string]bool{}
	for _, entry := range manifest.Entries {
		if paths[entry.Path] {
			return report, invalid
		}
		paths[entry.Path] = true
		if entry.Type != "file" {
			continue
		}
		if entry.Size < 0 || entry.Size > workspace.MaxFileBytes || !domain.ValidSHA256(entry.SHA256) {
			return report, invalid
		}
		total += entry.Size
		if total > workspace.MaxTotalBytes {
			return report, invalid
		}
		file, err := root.Open(entry.Path)
		if err != nil {
			return report, err
		}
		stat, statErr := file.Stat()
		body, readErr := io.ReadAll(io.LimitReader(file, entry.Size+1))
		closeErr := file.Close()
		if statErr != nil || readErr != nil || closeErr != nil || !stat.Mode().IsRegular() || int64(len(body)) != entry.Size || contextRefSHA(body) != entry.SHA256 {
			return report, invalid
		}
		want := ""
		switch entry.Path {
		case fixedA:
			want = GraphContentA
		case fixedB:
			want = GraphContentB
		case fixedJoined:
			want = GraphContentA + GraphContentB
		case fixedConflict:
			if string(body) != GraphContentA && string(body) != GraphContentB {
				return report, invalid
			}
			want = string(body)
		}
		if want != "" {
			if string(body) != want {
				return report, invalid
			}
			observedGraphResult = true
		}
	}
	if !observedGraphResult {
		return report, invalid
	}
	return report, nil
}
