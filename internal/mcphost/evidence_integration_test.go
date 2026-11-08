package mcphost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The client, CLI, fake worker and private owner are real subprocesses. All
// state and the disposable Git repository are confined to the test directory.
func TestEvidenceMCPRealOwnerMatchesCLIUnicodeAndDigest(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home, repo := t.TempDir(), t.TempDir()
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture binary: %v: %s", err, output)
	}
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture Git setup: %v: %s", err, output)
		}
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	root := filepath.Join(home, ".council", "v2")
	t.Cleanup(func() {
		desc, err := localstate.ReadDescriptor(root)
		if err != nil || desc.OwnerInstanceID == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "service", "stop", "--request-id", domain.NewID(), "--json")
		cmd.Env, cmd.Dir = env, repo
		_ = cmd.Run()
		for ctx.Err() == nil {
			lock, err := localstate.Acquire(root)
			if err == nil {
				_ = lock.Close()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("fixture owner did not release its lock")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cli := func(args ...string) json.RawMessage {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, append(args, "--json")...)
		cmd.Env, cmd.Dir = env, repo
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("fixture CLI failed: %v", err)
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *domain.Error   `json:"error"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope.Error != nil || len(envelope.Data) == 0 {
			t.Fatal("CLI did not return a successful owner envelope")
		}
		return envelope.Data
	}
	var opened struct {
		Project domain.Project `json:"project"`
	}
	if json.Unmarshal(cli("projects", "open", repo), &opened) != nil || !domain.ValidID(opened.Project.ID) {
		t.Fatal("invalid project fixture")
	}
	var admission domain.Admission
	if json.Unmarshal(cli("tasks", "start", "--project", opened.Project.ID, "--task", "Return the deterministic fake response", "--deliverable", "response_only", "--fake", "--timeout-seconds", "30"), &admission) != nil {
		t.Fatal("invalid task admission")
	}
	var attempt domain.Attempt
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		if json.Unmarshal(cli("attempts", "get", admission.Attempt.ID), &attempt) != nil {
			t.Fatal("invalid attempt")
		}
		if attempt.Phase == "settled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if attempt.Phase != "settled" || attempt.Outcome == nil || *attempt.Outcome != "succeeded" || attempt.ResponseArtifactID == nil {
		t.Fatal("fake response did not settle")
	}
	artifactID := *attempt.ResponseArtifactID
	command := exec.Command(binary, "mcp")
	command.Env, command.Dir = env, repo
	client := mcp.NewClient(&mcp.Implementation{Name: "council-evidence-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	defer session.Close()
	readPage := func(offset, through, limit string) evidence.Chunk {
		t.Helper()
		args := map[string]any{"id": artifactID, "offset": offset, "limit": limit}
		if through != "" {
			args["through"] = through
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "council_artifacts_bytes", Arguments: args})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("MCP byte read failed: %v", err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Status   int `json:"status"`
			Envelope struct {
				Data  evidence.Chunk `json:"data"`
				Error *domain.Error  `json:"error"`
			} `json:"envelope"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Status != 200 || envelope.Envelope.Error != nil {
			t.Fatal("invalid MCP byte response")
		}
		return envelope.Envelope.Data
	}
	var reconstructed []byte
	offset, through := "0", ""
	for range 256 {
		chunk := readPage(offset, through, "7")
		if chunk.ArtifactID != artifactID || chunk.Offset != offset || (through != "" && chunk.Through != through) {
			t.Fatal("page changed its captured identity or through boundary")
		}
		through = chunk.Through
		part, err := base64.StdEncoding.DecodeString(chunk.BytesBase64)
		if err != nil {
			t.Fatal(err)
		}
		reconstructed = append(reconstructed, part...)
		offset = chunk.NextOffset
		if chunk.EOF {
			if !chunk.StreamClosed {
				t.Fatal("settled stream reported open")
			}
			break
		}
	}
	if string(reconstructed) != fixture.ResponseText || through != strconv.Itoa(len(reconstructed)) || offset != through {
		t.Fatal("MCP pages changed or truncated the UTF-8 response")
	}
	var fromCLI evidence.Chunk
	if json.Unmarshal(cli("artifacts", "read", artifactID, "--offset", "0", "--through", through, "--limit", "65536"), &fromCLI) != nil {
		t.Fatal("invalid CLI byte response")
	}
	cliBytes, err := base64.StdEncoding.DecodeString(fromCLI.BytesBase64)
	if err != nil || !fromCLI.EOF || !bytes.Equal(cliBytes, reconstructed) {
		t.Fatal("CLI and MCP disagree on artifact bytes")
	}
	actual := sha256.Sum256(reconstructed)
	if fromCLI.SHA256 == nil || *fromCLI.SHA256 != hex.EncodeToString(actual[:]) {
		t.Fatal("owner digest does not match reconstructed bytes")
	}
	empty := readPage(through, through, "7")
	if !empty.EOF || empty.BytesBase64 != "" || empty.NextOffset != through {
		t.Fatal("empty terminal page changed")
	}

	// A real declared local check writes controlled binary data, pauses after an
	// observable durable prefix, then completes after this test releases it.
	// It runs under the same fake workflow owner and uses no provider process.
	helperDir := t.TempDir()
	helperSource := filepath.Join(helperDir, "main.go")
	helperBinary := filepath.Join(helperDir, "evidence-check")
	if runtime.GOOS == "windows" {
		helperBinary += ".exe"
	}
	const helperProgram = `package main
import ("bytes"; "os"; "time")
func main() {
 if len(os.Args)!=2 { os.Exit(2) }
 if _,err:=os.Stdout.Write(bytes.Repeat([]byte{0,255,195,169,10,66},60000)); err!=nil { os.Exit(3) }
 deadline:=time.Now().Add(45*time.Second)
 for { if _,err:=os.Stat(os.Args[1]); err==nil { break }; if time.Now().After(deadline) { os.Exit(4) }; time.Sleep(20*time.Millisecond) }
 if _,err:=os.Stdout.Write(bytes.Repeat([]byte{128,254,0,88,10},18000)); err!=nil { os.Exit(5) }
}`
	if err := os.WriteFile(helperSource, []byte(helperProgram), 0600); err != nil {
		t.Fatal(err)
	}
	helperBuild := exec.CommandContext(ctx, "go", "build", "-o", helperBinary, helperSource)
	helperBuild.Dir = repoRoot
	if output, err := helperBuild.CombinedOutput(); err != nil {
		t.Fatalf("build controlled check: %v: %s", err, output)
	}
	marker := filepath.Join(helperDir, "release")
	defer func() { _ = os.WriteFile(marker, []byte("release"), 0600) }()
	checkFile := filepath.Join(helperDir, "checks.json")
	checks, err := json.Marshal([]domain.CheckDeclaration{{Key: "binary-evidence", Executable: helperBinary, Arguments: []string{marker}}})
	if err != nil || os.WriteFile(checkFile, checks, 0600) != nil {
		t.Fatal("could not prepare controlled check")
	}
	var workflow domain.Admission
	if json.Unmarshal(cli("tasks", "start", "--project", opened.Project.ID, "--task", "Run the controlled local evidence check", "--fake", "--fake-scenario", "workflow-defect", "--checks-file", checkFile, "--timeout-seconds", "60"), &workflow) != nil {
		t.Fatal("invalid workflow admission")
	}
	var checkAttemptID string
	var stdoutArtifact domain.ArtifactMetadata
	checkDeadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(checkDeadline) {
		var attempts domain.Page[domain.AttemptSummary]
		if json.Unmarshal(cli("tasks", "attempts", workflow.Task.ID), &attempts) != nil {
			t.Fatal("invalid workflow attempts")
		}
		for _, candidate := range attempts.Items {
			if candidate.Role == "check" {
				checkAttemptID = candidate.ID
				break
			}
		}
		if checkAttemptID != "" {
			var artifacts domain.Page[domain.ArtifactMetadata]
			if json.Unmarshal(cli("attempts", "artifacts", checkAttemptID), &artifacts) != nil {
				t.Fatal("invalid check artifacts")
			}
			for _, artifact := range artifacts.Items {
				if artifact.Kind == "stdout" {
					stdoutArtifact = artifact
					break
				}
			}
			if n, err := strconv.Atoi(stdoutArtifact.ByteLength); err == nil && n >= 256<<10 && !stdoutArtifact.Closed {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if checkAttemptID == "" || stdoutArtifact.ID == "" || stdoutArtifact.Closed {
		t.Fatal("controlled check did not expose its growing stdout")
	}
	artifactID = stdoutArtifact.ID
	prefixPage := readPage("0", "", "7")
	prefixBound, err := strconv.Atoi(prefixPage.Through)
	if err != nil || prefixBound < 256<<10 || prefixPage.StreamClosed {
		t.Fatal("growing artifact did not expose the durable prefix")
	}
	if err := os.WriteFile(marker, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	var checkAttempt domain.Attempt
	for time.Now().Before(checkDeadline) {
		if json.Unmarshal(cli("attempts", "get", checkAttemptID), &checkAttempt) != nil {
			t.Fatal("invalid local check attempt")
		}
		if checkAttempt.Phase == "settled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if checkAttempt.Phase != "settled" || checkAttempt.Outcome == nil || *checkAttempt.Outcome != "succeeded" {
		t.Fatal("controlled check did not settle successfully")
	}
	oldBoundPage := readPage("0", prefixPage.Through, "65536")
	if oldBoundPage.Through != prefixPage.Through || !oldBoundPage.StreamClosed {
		t.Fatal("captured through changed when the stream grew")
	}
	expected := append(bytes.Repeat([]byte{0, 255, 195, 169, 10, 66}, 60000), bytes.Repeat([]byte{128, 254, 0, 88, 10}, 18000)...)
	if prefixBound >= len(expected) {
		t.Fatal("growing boundary did not precede final bytes")
	}
	var retainedPrefix []byte
	prefixOffset := "0"
	prefixEOF := false
	for range 32 {
		chunk := readPage(prefixOffset, prefixPage.Through, "65536")
		part, err := base64.StdEncoding.DecodeString(chunk.BytesBase64)
		if err != nil || chunk.Through != prefixPage.Through || chunk.Offset != prefixOffset {
			t.Fatal("captured prefix page changed")
		}
		retainedPrefix = append(retainedPrefix, part...)
		prefixOffset = chunk.NextOffset
		if chunk.EOF {
			prefixEOF = true
			break
		}
	}
	if !prefixEOF || prefixOffset != prefixPage.Through || !bytes.Equal(retainedPrefix, expected[:prefixBound]) {
		t.Fatal("captured prefix did not stop at its original boundary")
	}
	var binaryBytes []byte
	offset, through = "0", ""
	var finalDigest *string
	pages := 0
	for range 32 {
		chunk := readPage(offset, through, "65536")
		if chunk.Offset != offset || (through != "" && chunk.Through != through) {
			t.Fatal("binary page changed its fixed boundary")
		}
		through = chunk.Through
		part, err := base64.StdEncoding.DecodeString(chunk.BytesBase64)
		if err != nil || len(part) > 65536 {
			t.Fatal("invalid binary page")
		}
		var cliPage evidence.Chunk
		if json.Unmarshal(cli("artifacts", "read", artifactID, "--offset", offset, "--through", through, "--limit", "65536"), &cliPage) != nil {
			t.Fatal("invalid binary CLI page")
		}
		cliPart, err := base64.StdEncoding.DecodeString(cliPage.BytesBase64)
		if err != nil || !bytes.Equal(part, cliPart) || cliPage.NextOffset != chunk.NextOffset || cliPage.EOF != chunk.EOF {
			t.Fatal("CLI and MCP binary pages differ")
		}
		binaryBytes = append(binaryBytes, part...)
		pages++
		offset = chunk.NextOffset
		finalDigest = chunk.SHA256
		if chunk.EOF {
			break
		}
	}
	if pages < 2 || !bytes.Equal(binaryBytes, expected) || through != strconv.Itoa(len(expected)) || offset != through || prefixBound >= len(expected) {
		t.Fatal("binary or growing artifact bytes changed")
	}
	binaryDigest := sha256.Sum256(expected)
	if finalDigest == nil || *finalDigest != hex.EncodeToString(binaryDigest[:]) {
		t.Fatal("binary artifact digest changed")
	}
	var closedArtifacts domain.Page[domain.ArtifactMetadata]
	if json.Unmarshal(cli("attempts", "artifacts", checkAttemptID), &closedArtifacts) != nil {
		t.Fatal("invalid completed check artifacts")
	}
	var emptyArtifactID string
	for _, artifact := range closedArtifacts.Items {
		if artifact.Kind == "stderr" {
			emptyArtifactID = artifact.ID
			break
		}
	}
	if emptyArtifactID == "" {
		t.Fatal("controlled check has no stderr artifact")
	}
	artifactID = emptyArtifactID
	emptyStream := readPage("0", "", "7")
	var emptyCLI evidence.Chunk
	if json.Unmarshal(cli("artifacts", "read", artifactID, "--offset", "0", "--limit", "7"), &emptyCLI) != nil {
		t.Fatal("invalid empty CLI artifact")
	}
	emptyDigest := sha256.Sum256(nil)
	if !emptyStream.EOF || !emptyStream.StreamClosed || emptyStream.Through != "0" || emptyStream.NextOffset != "0" || emptyStream.BytesBase64 != "" || emptyStream.SHA256 == nil || *emptyStream.SHA256 != hex.EncodeToString(emptyDigest[:]) || emptyCLI.BytesBase64 != "" || !emptyCLI.EOF {
		t.Fatal("empty artifact differs between owner, MCP and CLI")
	}
}
