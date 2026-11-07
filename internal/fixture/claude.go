package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const claudeInternalArg = "__council_fixture_claude_v1"
const claudeChildArg = "__council_fixture_claude_child_v1"
const ClaudeContract = "003-v1"
const ClaudeResponseText = "\ufeffCouncil deterministic Claude fake response (003-v1).\r\nComplete fixture evidence: café, 東京, π, �.\r\nThis is a local fixture, not a provider result.\n"

// ArgsForHarness keeps Codex's response-file helper unchanged. For Claude the
// third argument is a private Council directory used only for helper markers;
// no response file is created, passed to a vendor or imported.
func ArgsForHarness(harnessID, scenario, auxiliaryPath string) ([]string, error) {
	if ValidGraphScenario(scenario) {
		response := auxiliaryPath
		if harnessID == "claude-code" {
			response = ""
		}
		return GraphArgs(harnessID, scenario, "author", response)
	}
	if harnessID == "" || harnessID == "codex" {
		return Args(scenario, auxiliaryPath)
	}
	if harnessID != "claude-code" || !filepath.IsAbs(auxiliaryPath) {
		return nil, errors.New("Select a supported fake harness and absolute private auxiliary directory.")
	}
	if scenario == "" {
		scenario = "response"
	}
	if !validClaudeScenario(scenario) {
		return nil, errors.New("Select a supported fixed Claude fake scenario.")
	}
	return []string{claudeInternalArg, "--scenario", scenario, "--auxiliary-root", auxiliaryPath}, nil
}

func validClaudeScenario(scenario string) bool {
	if ValidScenario(scenario) {
		return true
	}
	switch scenario {
	case "trailing_observation", "malformed", "missing_result", "repeated_result", "trailing_error", "success_error", "model_usage", "empty_response", "empty_candidate":
		return true
	}
	return false
}

func handleClaudeInternal(args []string) (bool, int) {
	if len(args) > 0 && args[0] == claudeChildArg {
		if len(args) != 2 || !filepath.IsAbs(args[1]) || filepath.Base(args[1]) != "owned-descendant.marker" || !strings.HasPrefix(filepath.Base(filepath.Dir(args[1])), "claude-fixture-") {
			return true, 2
		}
		code := runChild(args[1:])
		os.Remove(args[1])
		os.Remove(filepath.Dir(args[1]))
		return true, code
	}
	if len(args) == 0 || args[0] != claudeInternalArg {
		return false, 0
	}
	if len(args) != 5 || args[1] != "--scenario" || args[3] != "--auxiliary-root" || !validClaudeScenario(args[2]) || !filepath.IsAbs(args[4]) {
		return true, 2
	}
	return true, runClaude(args[2], args[4])
}

func emitClaude(kind string, fields map[string]any) error {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["type"] = kind
	fields["council_fixture"] = true
	fields["fixture_contract"] = ClaudeContract
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(encoded, '\n'))
	return err
}

func runClaude(scenario, auxiliaryRoot string) int {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 256*1024+1))
	if err != nil || len(input) > 256*1024 {
		return 2
	}
	sum := sha256.Sum256(input)
	if emitClaude("system", map[string]any{"subtype": "init", "session_id": "council-claude-fixture-session", "model": "fixture-configured-alias", "claude_code_version": "2.1.318", "permissionMode": "bypassPermissions", "fixture_input_sha256": hex.EncodeToString(sum[:])}) != nil {
		return 3
	}
	if scenario == "malformed" {
		fmt.Fprintln(os.Stdout, `{"type":"result",`)
		return 0
	}
	if scenario == "slow" {
		time.Sleep(30 * time.Second)
	}
	if scenario == "late_success" {
		time.Sleep(time.Second)
	}
	if scenario == "owned_descendant" {
		private, err := os.MkdirTemp(auxiliaryRoot, "claude-fixture-")
		if err != nil {
			return 3
		}
		// A killed fake may leave this labelled private diagnostic directory. It
		// never changes the managed source or acts as a final-response artifact.
		marker := filepath.Join(private, "owned-descendant.marker")
		executable, err := os.Executable()
		if err != nil {
			return 3
		}
		child := exec.Command(executable, claudeChildArg, marker)
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		if child.Start() != nil {
			return 3
		}
		go child.Wait()
		deadline := time.Now().Add(3 * time.Second)
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			if time.Now().After(deadline) {
				return 3
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if scenario == "candidate" || scenario == "partial_failure" || scenario == "empty_candidate" {
		if os.WriteFile("council-fixture.txt", []byte(CandidateText), 0600) != nil {
			return 3
		}
	}
	if scenario == "cleanup_unknown" && emitClaude("council.fixture.cleanup_unknown", nil) != nil {
		return 3
	}
	text := ClaudeResponseText
	if scenario == "partial_failure" {
		text = PartialText
	}
	if emitClaude("assistant", map[string]any{"session_id": "council-claude-fixture-session", "parent_tool_use_id": nil, "message": map[string]any{"id": "fixture-assistant", "model": "fixture-main-model", "content": []map[string]any{{"type": "text", "text": text}}}}) != nil {
		return 3
	}
	fmt.Fprintln(os.Stderr, "Council deterministic Claude fixture stderr tail (003-v1).")
	if scenario == "missing_result" {
		return 0
	}
	if scenario == "partial_failure" {
		_ = emitClaude("result", map[string]any{"session_id": "council-claude-fixture-session", "subtype": "error_during_execution", "is_error": true, "errors": []string{"The local Claude fixture failed after partial evidence."}})
		return 17
	}
	if scenario == "empty_response" || scenario == "empty_candidate" {
		text = ""
	}
	result := map[string]any{"session_id": "council-claude-fixture-session", "subtype": "success", "is_error": scenario == "success_error", "result": text, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0}, "modelUsage": map[string]any{"fixture-main-model": map[string]any{"inputTokens": 0, "outputTokens": 0}}}
	if scenario == "model_usage" {
		result["modelUsage"] = map[string]any{"fixture-main-model": map[string]any{"inputTokens": 12, "outputTokens": 4, "costUSD": 0.001}, "fixture-helper-model": map[string]any{"inputTokens": 2, "outputTokens": 1}}
	}
	if emitClaude("result", result) != nil {
		return 3
	}
	switch scenario {
	case "repeated_result":
		if err := emitClaude("result", result); err != nil {
			return 3
		}
	case "trailing_error":
		if emitClaude("assistant", map[string]any{"session_id": "council-claude-fixture-session", "parent_tool_use_id": nil, "error": "authentication_failed", "message": map[string]any{"model": "fixture-main-model", "content": []map[string]string{{"type": "text", "text": "Illustrative trailing native error."}}}}) != nil {
			return 3
		}
	case "trailing_observation":
		if emitClaude("system", map[string]any{"subtype": "session_writes_lost", "session_id": "council-claude-fixture-session", "fixture_tail": "retained after result"}) != nil {
			return 3
		}
	}
	return 0
}
