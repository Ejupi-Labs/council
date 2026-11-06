package fixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workflow"
)

const workflowInternalArg = "__council_workflow_fixture_v1"
const WorkflowFile = "council-workflow.txt"
const WorkflowInvalid = "Council controlled workflow validation: INVALID\r\n"
const WorkflowValid = "Council controlled workflow validation: VALID\r\n"
const WorkflowResponse = "Council controlled local workflow fixture (005-v1).\r\nNo provider/model/account usage was measured.\r\n"

// WorkflowArgs is controller-selected. Public fake scenarios cannot name a role.
func WorkflowArgs(harnessID, scenario, role, responsePath string) ([]string, error) {
	if harnessID == "" {
		harnessID = "codex"
	}
	if harnessID != "codex" && harnessID != "claude-code" {
		return nil, fmt.Errorf("Unsupported fixed workflow harness.")
	}
	valid := role == "author" && (scenario == "workflow-document" || scenario == "workflow-defect") || role == "review" && (scenario == "workflow-review-blocking" || scenario == "workflow-review-clean") || role == "correction" && scenario == "workflow-correction"
	if !valid || harnessID == "codex" && !filepath.IsAbs(responsePath) || harnessID == "claude-code" && responsePath != "" {
		return nil, fmt.Errorf("Select a fixed scenario/role and its supported output contract.")
	}
	return []string{workflowInternalArg, "--harness", harnessID, "--scenario", scenario, "--role", role, "--output-path", responsePath}, nil
}

// HandleWorkflowInternal is wired before ordinary CLI dispatch by the entrypoint.
func HandleWorkflowInternal(args []string) (bool, int) {
	if len(args) > 0 && args[0] == "__fixture" {
		if len(args) == 4 && args[1] == "check" && args[2] == "--case" && args[3] == "workflow-validation" {
			data, err := os.ReadFile(WorkflowFile)
			fmt.Fprintln(os.Stdout, "Council fixed local verifier (005-v1).")
			if err != nil {
				fmt.Fprintln(os.Stderr, "Controlled validation file is unavailable.")
				return true, 2
			}
			if string(data) != WorkflowValid {
				fmt.Fprintln(os.Stderr, "Observed controlled validation defect.")
				return true, 1
			}
			return true, 0
		}
		if len(args) == 6 && args[1] == "publish" && args[2] == "--case" && args[3] == "workflow-publication" && args[4] == "--destination" && filepath.IsAbs(args[5]) {
			f, err := os.OpenFile(args[5], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return true, 2
			}
			_, writeErr := io.WriteString(f, "Council controlled local publication marker (005-v1).\n")
			syncErr := f.Sync()
			closeErr := f.Close()
			if writeErr != nil || syncErr != nil || closeErr != nil {
				return true, 3
			}
			fmt.Fprintln(os.Stdout, "Controlled local command completed; no remote publication is attested.")
			return true, 0
		}
		return true, 2
	}
	if len(args) == 0 || args[0] != workflowInternalArg {
		return false, 0
	}
	if len(args) != 9 || args[1] != "--harness" || args[3] != "--scenario" || args[5] != "--role" || args[7] != "--output-path" {
		return true, 2
	}
	if _, err := WorkflowArgs(args[2], args[4], args[6], args[8]); err != nil {
		return true, 2
	}
	return true, runWorkflow(args[2], args[4], args[6], args[8])
}

type workflowEvidence struct {
	Ref   string   `json:"ref"`
	Path  string   `json:"path"`
	Scope string   `json:"scope"`
	Paths []string `json:"paths"`
}
type workflowPrompt struct {
	Original         string                 `json:"originalInstructions"`
	Guidance         string                 `json:"guidance"`
	Acceptance       domain.TaskAcceptance  `json:"acceptance"`
	Role             string                 `json:"role"`
	Subject          domain.ReviewSubject   `json:"subject"`
	ConditionsSHA256 string                 `json:"conditionsSHA256"`
	Evidence         []workflowEvidence     `json:"evidence"`
	Findings         []domain.ReviewFinding `json:"findings"`
	PriorFindingIDs  []string               `json:"priorFindingIds"`
}

func runWorkflow(harness, scenario, role, responsePath string) int {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, domain.MaxPromptBytes+1))
	if err != nil || len(input) > domain.MaxPromptBytes {
		return 2
	}
	start := bytes.IndexByte(input, '{')
	if start < 0 {
		return 2
	}
	var prompt workflowPrompt
	if err = json.Unmarshal(input[start:], &prompt); err != nil {
		return 2
	}
	var structured *domain.ReviewReport
	if role == "author" && scenario == "workflow-document" {
		op, ok := workflow.RecognizeDocument(prompt.Original, domain.TaskVersion{GuidanceText: prompt.Guidance, Acceptance: prompt.Acceptance})
		if !ok {
			return 3
		}
		data, e := readWorkflowFile(op.Path)
		if e != nil || bytes.Count(data, []byte(op.Old)) != 1 {
			return 3
		}
		if e = os.WriteFile(filepath.FromSlash(op.Path), bytes.Replace(data, []byte(op.Old), []byte(op.New), 1), 0600); e != nil {
			return 3
		}
	} else if role == "author" && scenario == "workflow-defect" {
		if err = os.WriteFile(WorkflowFile, []byte(WorkflowInvalid), 0600); err != nil {
			return 3
		}
	} else if role == "correction" {
		data, e := readWorkflowFile(WorkflowFile)
		if e != nil || string(data) != WorkflowInvalid || len(prompt.Findings) == 0 {
			return 3
		}
		if e = os.WriteFile(WorkflowFile, []byte(WorkflowValid), 0600); e != nil {
			return 3
		}
	} else if role == "review" {
		report, e := workflowReview(prompt)
		if e != nil {
			return 3
		}
		structured = &report
	}
	if harness == "claude-code" {
		if emitClaude("system", map[string]any{"subtype": "init", "session_id": "council-workflow-fixture"}) != nil {
			return 4
		}
		fields := map[string]any{"subtype": "success", "is_error": false, "session_id": "council-workflow-fixture", "result": WorkflowResponse}
		if structured != nil {
			fields["structured_output"] = structured
		}
		if emitClaude("result", fields) != nil {
			return 4
		}
	} else {
		text := WorkflowResponse
		if structured != nil {
			raw, e := json.Marshal(structured)
			if e != nil {
				return 4
			}
			text = string(raw)
		}
		if writeResponse(responsePath, text) != nil {
			return 4
		}
		if emit("thread.started", map[string]any{"thread_id": "council-workflow-fixture"}) != nil || emit("turn.started", nil) != nil || emit("item.completed", map[string]any{"item": map[string]any{"id": "workflow-response", "type": "agent_message", "text": text}}) != nil || emit("turn.completed", map[string]any{"usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}) != nil {
			return 4
		}
	}
	return 0
}

// This fixed reviewer observes the supplied evidence and its actual controlled
// candidate/response. Its verdict is not a general-purpose model-quality claim.
func workflowReview(prompt workflowPrompt) (domain.ReviewReport, error) {
	report := domain.ReviewReport{SchemaVersion: domain.ReviewContract, Subject: prompt.Subject, ConditionsSHA256: prompt.ConditionsSHA256, Outcome: "approved", InspectedEvidence: []domain.ReviewInspection{}, Findings: []domain.ReviewFinding{}, Gaps: []domain.ReviewGap{}, ResolvedFindingIDs: []string{}}
	responseObserved := false
	for _, reference := range prompt.Evidence {
		file, err := os.Open(reference.Path)
		if err != nil {
			return report, err
		}
		stat, statErr := file.Stat()
		data, readErr := io.ReadAll(io.LimitReader(file, (64<<20)+1))
		closeErr := file.Close()
		if statErr != nil || readErr != nil || closeErr != nil || !stat.Mode().IsRegular() || len(data) > 64<<20 {
			return report, fmt.Errorf("Controlled supplied evidence is unavailable.")
		}
		report.InspectedEvidence = append(report.InspectedEvidence, domain.ReviewInspection{Ref: reference.Ref, Scope: reference.Scope, Paths: append([]string{}, reference.Paths...)})
		if prompt.Subject.Kind == "response" && reference.Ref == "selected-response" && reference.Scope == "whole_response" {
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != prompt.Subject.SHA256 || string(data) != ResponseText && string(data) != ClaudeResponseText && string(data) != WorkflowResponse {
				return report, fmt.Errorf("Controlled response bytes do not match the exact supplied subject.")
			}
			responseObserved = true
		}
	}
	if len(report.InspectedEvidence) == 0 {
		return report, fmt.Errorf("Controlled review requires actual supplied evidence.")
	}
	if prompt.Subject.Kind == "response" {
		if !responseObserved {
			return report, fmt.Errorf("Controlled response evidence is missing.")
		}
		return report, nil
	}
	if prompt.Subject.Kind != "candidate" {
		return report, fmt.Errorf("Unsupported controlled review subject.")
	}
	if _, err := os.Lstat(WorkflowFile); err == nil {
		data, readErr := readWorkflowFile(WorkflowFile)
		if readErr != nil {
			return report, readErr
		}
		switch string(data) {
		case WorkflowInvalid:
			report.Outcome = "changes_required"
			report.Findings = append(report.Findings, domain.ReviewFinding{Key: "controlled-validation-defect", Severity: "blocking", ConditionKeys: []string{"task-instructions"}, Locations: []domain.ReviewLocation{{Path: WorkflowFile, StartLine: 1, EndLine: 1}}, Summary: "The actual retained candidate contains INVALID instead of the required VALID marker.", EvidenceRefs: []string{prompt.Evidence[0].Ref}})
		case WorkflowValid:
			report.ResolvedFindingIDs = append([]string{}, prompt.PriorFindingIDs...)
		default:
			return report, fmt.Errorf("Unrecognized controlled candidate bytes.")
		}
		return report, nil
	} else if !os.IsNotExist(err) {
		return report, err
	}
	// Explicit review can also qualify the documented fixed document fixture;
	// it must inspect the changed file, rather than approving from its scenario.
	op, ok := workflow.RecognizeDocument(prompt.Original, domain.TaskVersion{})
	if !ok || prompt.Guidance != "" || prompt.Acceptance.ConditionsText != "" {
		return report, fmt.Errorf("No supported controlled candidate review.")
	}
	data, err := readWorkflowFile(op.Path)
	if err != nil || bytes.Contains(data, []byte(op.Old)) || bytes.Count(data, []byte(op.New)) != 1 {
		return report, fmt.Errorf("Controlled document candidate did not contain the required actual replacement.")
	}
	return report, nil
}

func readWorkflowFile(relative string) ([]byte, error) {
	if filepath.IsAbs(relative) || strings.ContainsAny(relative, "\\\x00") || strings.Contains(relative, "..") {
		return nil, fmt.Errorf("Unsafe controlled fixture path.")
	}
	parts := strings.Split(relative, "/")
	for i := range parts {
		info, err := os.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Unavailable ordinary controlled fixture input.")
		}
	}
	return os.ReadFile(filepath.FromSlash(relative))
}
