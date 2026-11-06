package process

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

const inputDeliveryTestArg = "__council_input_delivery_test_v1"
const fullInputDeliveryTestArg = "__council_full_input_delivery_test_v1"

// This private test-only entrypoint is intercepted before TestMain. It never
// becomes a public fake scenario or a vendor executable.
func init() {
	if len(os.Args) == 2 && os.Args[1] == fullInputDeliveryTestArg {
		input, err := io.ReadAll(io.LimitReader(os.Stdin, 256*1024+1))
		if err != nil || len(input) > 256*1024 {
			os.Exit(17)
		}
		sum := sha256.Sum256(input)
		fmt.Fprintf(os.Stdout, "{\"type\":\"turn.completed\",\"council_fixture\":true,\"fixture_input_bytes\":%d,\"fixture_input_sha256\":\"%s\"}\n", len(input), hex.EncodeToString(sum[:]))
		os.Exit(0)
	}
	if len(os.Args) == 2 && os.Args[1] == inputDeliveryTestArg {
		prefix := make([]byte, 1)
		n, _ := io.ReadFull(os.Stdin, prefix)
		os.Stdin.Close()
		fmt.Fprintf(os.Stdout, "{\"type\":\"turn.completed\",\"council_fixture\":true,\"fixture_prefix_read\":%d}\n", n)
		os.Exit(0)
	}
}

func TestRootZeroAndTerminalCannotHideShortOriginalInputWrite(t *testing.T) {
	self, _ := os.Executable()
	input := bytes.Repeat([]byte("x"), 256*1024)
	copy(input[len(input)-48:], []byte("REQUIRED original constraint at the end of input"))
	var output bytes.Buffer
	o, err := Prepare(context.Background(), NativeSpec{Executable: self, Args: []string{inputDeliveryTestArg}, Cwd: t.TempDir(), Stdin: input, Stdout: &output, Stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	root := waitRoot(t, o)
	if !root.Known || root.ExitCode != 0 {
		t.Fatalf("fixture did not report its deliberately misleading root success: %+v", root)
	}
	ctx, cancel := context.WithTimeout(context.Background(), CleanupLimit)
	defer cancel()
	result := o.Cleanup(ctx)
	if !result.Verified || !result.StdoutDrained || !result.StderrDrained {
		t.Fatalf("native scope or output did not settle: %+v", result)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"type":"turn.completed"`)) || !bytes.Contains(output.Bytes(), []byte(`"fixture_prefix_read":1`)) {
		t.Fatal("fixture did not actually consume a prefix and emit valid terminal bytes")
	}
	if result.StdinState != StdinIncomplete || !result.StdinError || result.ExpectedInputBytes != int64(len(input)) || result.StdinBytesWritten >= result.ExpectedInputBytes {
		t.Fatalf("known short input was reported complete despite root0: %+v", result)
	}
	select {
	case fault := <-o.Faults():
		if fault.Stream != "stdin" {
			t.Fatal("input fault was not retained distinctly")
		}
	case <-time.After(time.Second):
		t.Fatal("known input error was not observable")
	}
}

func TestZeroLengthStartedInputHasCompleteDeliveryProof(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	waitRoot(t, o)
	result := cleanupOwned(t, o)
	if result.StdinState != StdinComplete || result.StdinError || result.StdinBytesWritten != 0 || result.ExpectedInputBytes != 0 {
		t.Fatalf("zero-length input became incomplete: %+v", result)
	}
}

func TestPreGOCleanupRetainsNotStartedInputWithoutFailure(t *testing.T) {
	s := selfSpec(t, "linger")
	s.Stdin = []byte("retained original input")
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	result := cleanupOwned(t, o)
	if result.StdinState != StdinNotStarted || result.StdinError || result.StdinBytesWritten != 0 || result.ExpectedInputBytes != int64(len(s.Stdin)) {
		t.Fatalf("unreleased launch gate fabricated input failure: %+v", result)
	}
	select {
	case fault := <-o.Faults():
		if fault.Stream == "stdin" {
			t.Fatal("pre-GO cancellation emitted an input failure")
		}
	default:
	}
}

func TestInputProofCompletesWithSuccessfulFullWrite(t *testing.T) {
	input := append([]byte("\ufeff"), bytes.Repeat([]byte("Exact café 東京 �\r\n"), 4096)...)
	self, _ := os.Executable()
	var output bytes.Buffer
	o, err := Prepare(context.Background(), NativeSpec{Executable: self, Args: []string{fullInputDeliveryTestArg}, Cwd: t.TempDir(), Stdin: input, Stdout: &output, Stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	root := waitRoot(t, o)
	if !root.Known || root.ExitCode != 0 {
		t.Fatalf("complete-input fixture failed: %+v", root)
	}
	ctx, cancel := context.WithTimeout(context.Background(), CleanupLimit)
	defer cancel()
	result := o.Cleanup(ctx)
	if result.StdinState != StdinComplete || result.StdinError || result.StdinBytesWritten != int64(len(input)) || result.ExpectedInputBytes != int64(len(input)) || !result.StdoutDrained {
		t.Fatalf("full kernel pipe write lost its proof: %+v", result)
	}
	var observed struct {
		Bytes int    `json:"fixture_input_bytes"`
		Hash  string `json:"fixture_input_sha256"`
	}
	if json.Unmarshal(output.Bytes(), &observed) != nil {
		t.Fatal("fixture input proof was not valid JSON")
	}
	expected := sha256.Sum256(input)
	if observed.Bytes != len(input) || observed.Hash != hex.EncodeToString(expected[:]) {
		t.Fatal("native fixture did not consume the exact complete input")
	}
}
