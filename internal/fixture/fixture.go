// Package fixture implements explicit deterministic local fixtures. It is never a vendor fallback.
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
	"strconv"
	"time"
)

const InternalArg = "__council_fixture_v1"
const childArg = "__council_fixture_child_v1"
const Contract = "002-v1"
const ResponseText = "Council deterministic fake response (002-v1).\nComplete fixture evidence: café, 東京, π.\nThis is a local fixture, not a provider result.\n"
const CandidateText = "Council deterministic fake candidate (002-v1).\n"
const PartialText = "Council deterministic fake partial response (002-v1).\nThe fixture failed after retaining this evidence.\n"

func ValidScenario(s string) bool {
	switch s {
	case "response", "candidate", "partial_failure", "slow", "late_success", "owned_descendant", "cleanup_unknown":
		return true
	}
	return false
}
func Args(scenario, responsePath string) ([]string, error) {
	if scenario == "" {
		scenario = "response"
	}
	if !ValidScenario(scenario) || !filepath.IsAbs(responsePath) {
		return nil, errors.New("Select a supported fake scenario and an absolute response artifact path.")
	}
	return []string{InternalArg, "--scenario", scenario, "--response-path", responsePath}, nil
}

func emit(kind string, fields map[string]any) error {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["type"] = kind
	fields["council_fixture"] = true
	fields["fixture_contract"] = Contract
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(data, '\n'))
	return err
}

func HandleInternal(args []string) (bool, int) {
	if handled, code := handleGraphInternal(args); handled {
		return true, code
	}
	if handled, code := handleClaudeInternal(args); handled {
		return true, code
	}
	if len(args) == 0 {
		return false, 0
	}
	if args[0] == childArg {
		return true, runChild(args[1:])
	}
	if args[0] != InternalArg {
		return false, 0
	}
	if len(args) == 2 && args[1] == "--version" {
		fmt.Fprintln(os.Stdout, "codex-cli 0.0.0-council-fixture-002-v1")
		return true, 0
	}
	if len(args) != 5 || args[1] != "--scenario" || args[3] != "--response-path" || !ValidScenario(args[2]) || !filepath.IsAbs(args[4]) {
		return true, 2
	}
	return true, run(args[2], args[4])
}

func writeResponse(path, text string) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(path)
	flags := os.O_CREATE | os.O_EXCL | os.O_WRONLY
	if info, err := root.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("Fixture response target is not an ordinary file.")
		}
		flags = os.O_WRONLY | os.O_TRUNC
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return err
	}
	_, writeErr := io.WriteString(f, text)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func run(scenario, responsePath string) int {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 256*1024+1))
	if err != nil || len(input) > 256*1024 {
		return 2
	}
	sum := sha256.Sum256(input)
	if emit("thread.started", map[string]any{"thread_id": "council-fixture-thread"}) != nil {
		return 3
	}
	if emit("turn.started", map[string]any{"fixture_input_sha256": hex.EncodeToString(sum[:])}) != nil {
		return 3
	}
	if scenario == "slow" {
		time.Sleep(30 * time.Second)
	}
	if scenario == "late_success" {
		time.Sleep(time.Second)
	}
	if scenario == "owned_descendant" {
		executable, err := os.Executable()
		if err != nil {
			return 3
		}
		child := exec.Command(executable, childArg, filepath.Join(filepath.Dir(responsePath), "fixture-descendant.marker"))
		child.Stdin = os.Stdin
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			return 3
		}
		go child.Wait()
		// Wait for its first in-scope instruction so root-first completion is deterministic.
		deadline := time.Now().Add(3 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(filepath.Dir(responsePath), "fixture-descendant.marker")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				return 3
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if scenario == "candidate" || scenario == "partial_failure" {
		if err := os.WriteFile("council-fixture.txt", []byte(CandidateText), 0600); err != nil {
			return 3
		}
	}
	text := ResponseText
	if scenario == "partial_failure" {
		text = PartialText
	}
	if err := writeResponse(responsePath, text); err != nil {
		return 3
	}
	if scenario == "cleanup_unknown" {
		if emit("council.fixture.cleanup_unknown", nil) != nil {
			return 3
		}
	}
	if emit("item.completed", map[string]any{"item": map[string]any{"id": "fixture-message", "type": "agent_message", "text": text}}) != nil {
		return 3
	}
	fmt.Fprintln(os.Stderr, "Council deterministic fixture stderr tail (002-v1).")
	if scenario == "partial_failure" {
		_ = emit("turn.failed", map[string]any{"error": map[string]any{"message": "The deterministic fixture failed after producing partial evidence."}})
		return 17
	}
	if emit("turn.completed", map[string]any{"usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "cached_input_tokens": 0}}) != nil {
		return 3
	}
	return 0
}
func runChild(args []string) int {
	if len(args) != 1 || !filepath.IsAbs(args[0]) {
		return 2
	}
	deadline := time.Now().Add(30 * time.Second)
	for n := 0; time.Now().Before(deadline); n++ {
		if os.WriteFile(args[0], []byte(strconv.Itoa(os.Getpid())+":"+strconv.Itoa(n)), 0600) != nil {
			return 3
		}
		time.Sleep(25 * time.Millisecond)
	}
	return 0
}
