package fixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const graphInternalArg = "__council_graph_fixture_v1"
const GraphFileA = "council-graph-a.txt"
const GraphFileB = "council-graph-b.txt"
const GraphJoinedFile = "council-graph-joined.txt"
const GraphConflictFile = "council-graph-conflict.txt"
const GraphContentA = "Council controlled graph branch A: café, 東京.\r\n"
const GraphContentB = "Council controlled graph branch B: π, 🙂.\r\n"
const GraphResponse = "Council controlled task graph fixture (006-v1).\r\nNo live provider, account or usage was measured.\r\n"
const GraphPartialResponse = "Council controlled task graph failure (006-v1).\r\nUseful partial bytes are retained; no successful outcome is attested.\r\n"
const GraphReadyFile = "council-graph-ready.marker"
const GraphReleaseFile = "council-graph-release.marker"

func ValidGraphScenario(s string) bool {
	switch s {
	case "graph-branch-a", "graph-branch-b", "graph-join", "graph-join-no-edit", "graph-fail", "graph-wait", "graph-response", "graph-document", "graph-diverge-a", "graph-diverge-b":
		return true
	}
	return false
}

// GraphArgs selects only fixed local actions. It never parses task prose as a
// command, accepts user scripts or invokes an installed vendor executable.
func GraphArgs(harness, scenario, role, response string) ([]string, error) {
	if harness == "" {
		harness = "codex"
	}
	if harness != "codex" && harness != "claude-code" || role != "author" || !ValidGraphScenario(scenario) || harness == "codex" && !filepath.IsAbs(response) || harness == "claude-code" && response != "" {
		return nil, errors.New("Select a fixed supported graph fixture and its native output contract.")
	}
	return []string{graphInternalArg, "--harness", harness, "--scenario", scenario, "--role", role, "--output-path", response}, nil
}

func handleGraphInternal(args []string) (bool, int) {
	if len(args) == 0 || args[0] != graphInternalArg {
		return false, 0
	}
	if (len(args) != 9 && len(args) != 11 && len(args) != 13) || args[1] != "--harness" || args[3] != "--scenario" || args[5] != "--role" || args[7] != "--output-path" {
		return true, 2
	}
	if len(args) == 13 && args[4] == "graph-review" && args[6] == "review" && args[9] == "--context-directory" && args[11] == "--workspace-directory" {
		if _, err := GraphReviewArgsWithContext(args[2], args[8], args[10], args[12]); err != nil {
			return true, 2
		}
		return true, runGraphReview(args[2], args[8], args[10], args[12])
	}
	if len(args) == 13 {
		return true, 2
	}
	if _, err := GraphArgs(args[2], args[4], args[6], args[8]); err != nil {
		return true, 2
	}
	if len(args) == 11 {
		if args[9] != "--context-directory" {
			return true, 2
		}
		if _, err := GraphArgsWithContext(args[2], args[4], args[6], args[8], args[10]); err != nil {
			return true, 2
		}
		return true, runGraphWithContext(args[2], args[4], args[8], args[10])
	}
	return true, runGraph(args[2], args[4], args[8])
}

func writeGraphFile(name string, body []byte) error {
	root, err := os.OpenRoot(".")
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	return errors.Join(writeErr, file.Sync(), file.Close())
}

func graphAction(scenario string) error {
	switch scenario {
	case "graph-response":
		return nil
	case "graph-document":
		// This fixture replaces one fixed owned file and exact literal bytes. It
		// never interprets the task instruction as a path, script or command.
		root, err := os.OpenRoot(".")
		if err != nil {
			return err
		}
		defer root.Close()
		before, err := root.Lstat("source.txt")
		if err != nil || !before.Mode().IsRegular() {
			return errors.New("The fixed ordinary document input is unavailable.")
		}
		file, err := root.OpenFile("source.txt", os.O_RDWR, 0)
		if err != nil {
			return err
		}
		actual, statErr := file.Stat()
		body, readErr := io.ReadAll(io.LimitReader(file, 1024))
		if statErr != nil || readErr != nil || !actual.Mode().IsRegular() || !os.SameFile(before, actual) || !bytes.Equal(body, []byte("original\n")) {
			return errors.Join(errors.New("The exact fixed document before bytes are unavailable."), file.Close())
		}
		updated := []byte("updated\n")
		written, writeErr := file.WriteAt(updated, 0)
		if written != len(updated) && writeErr == nil {
			writeErr = io.ErrShortWrite
		}
		if writeErr != nil {
			return errors.Join(writeErr, file.Close())
		}
		return errors.Join(file.Truncate(int64(len(updated))), file.Sync(), file.Close())
	case "graph-branch-a", "graph-fail":
		return writeGraphFile(GraphFileA, []byte(GraphContentA))
	case "graph-branch-b":
		return writeGraphFile(GraphFileB, []byte(GraphContentB))
	case "graph-diverge-a":
		return writeGraphFile(GraphConflictFile, []byte(GraphContentA))
	case "graph-diverge-b":
		return writeGraphFile(GraphConflictFile, []byte(GraphContentB))
	case "graph-join", "graph-join-no-edit":
		a, first := readWorkflowFile(GraphFileA)
		b, second := readWorkflowFile(GraphFileB)
		if first != nil || second != nil || !bytes.Equal(a, []byte(GraphContentA)) || !bytes.Equal(b, []byte(GraphContentB)) {
			return errors.New("Both exact controlled predecessor files are required.")
		}
		if scenario == "graph-join" {
			return writeGraphFile(GraphJoinedFile, append(a, b...))
		}
		return nil
	case "graph-wait":
		if err := writeGraphFile(GraphReadyFile, []byte("ready\n")); err != nil {
			return err
		}
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			body, err := readWorkflowFile(GraphReleaseFile)
			if err == nil && string(body) == "release\n" {
				if err := os.Remove(GraphReadyFile); err != nil {
					return err
				}
				if err := os.Remove(GraphReleaseFile); err != nil {
					return err
				}
				return writeGraphFile(GraphFileA, []byte(GraphContentA))
			}
			time.Sleep(5 * time.Millisecond)
		}
		return errors.New("The controlled graph release was not observed within its fixed bound.")
	}
	return errors.New("Unsupported controlled graph action.")
}

func runGraph(harness, scenario, response string) int {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, domain.MaxPromptBytes+1))
	if err != nil || len(input) > domain.MaxPromptBytes {
		return 2
	}
	return runGraphInput(harness, scenario, response, input, nil)
}

func runGraphInput(harness, scenario, response string, input []byte, reads []GraphContextInspection) int {
	digest := sha256.Sum256(input)
	turn := map[string]any{"fixture_input_sha256": hex.EncodeToString(digest[:])}
	if reads != nil {
		turn["fixture_context_reads"] = reads
	}
	if harness == "claude-code" {
		turn["subtype"], turn["session_id"] = "init", "council-graph-fixture"
		if emitClaude("system", turn) != nil {
			return 3
		}
	} else if emit("thread.started", map[string]any{"thread_id": "council-graph-fixture"}) != nil || emit("turn.started", turn) != nil {
		return 3
	}
	if err := graphAction(scenario); err != nil {
		fmt.Fprintln(os.Stderr, "Controlled graph prerequisites or local action failed.")
		return 9
	}
	text, failed := GraphResponse, scenario == "graph-fail"
	if failed {
		text = GraphPartialResponse
	}
	if harness == "claude-code" {
		subtype := "success"
		if failed {
			subtype = "error_during_execution"
		}
		if emitClaude("result", map[string]any{"subtype": subtype, "is_error": failed, "session_id": "council-graph-fixture", "result": text}) != nil {
			return 4
		}
	} else {
		if writeResponse(response, text) != nil || emit("item.completed", map[string]any{"item": map[string]any{"id": "graph-response", "type": "agent_message", "text": text}}) != nil {
			return 4
		}
		if failed {
			if emit("turn.failed", map[string]any{"error": map[string]any{"message": "Controlled graph failure after retaining useful bytes."}}) != nil {
				return 4
			}
		} else if emit("turn.completed", nil) != nil {
			return 4
		}
	}
	if failed {
		return 17
	}
	return 0
}
