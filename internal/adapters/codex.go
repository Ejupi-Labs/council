package adapters

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func UnknownObservations(mode string) domain.TargetObservations {
	unknown := domain.ObservedFact{Status: "unknown"}
	return domain.TargetObservations{Provider: unknown, Account: unknown, Model: unknown, NativeSession: unknown, Permissions: domain.PermissionObservation{RequestedMode: mode, Limitations: []string{"Native permission settings are harness-specific; no universal OS sandbox is provided."}}, Usage: domain.UsageObservation{Completeness: "unknown"}}
}

func BuildCodex(ctx context.Context, original string, version domain.TaskVersion, responsePath string, resolve Resolver) (Launch, error) {
	if err := domain.ValidateModelArgument(version.Pins.Model); err != nil {
		return Launch{}, err
	}
	mode := version.RequestedPolicy.Mode
	if version.TaskPolicyOverride != nil {
		mode = *version.TaskPolicyOverride
	}
	if mode != "autonomous" {
		return Launch{}, domain.Fail("permission_unavailable", "This native execution contract supports autonomous full access. Supervised tool approval is unavailable.")
	}
	if version.Pins.HarnessID != "" && version.Pins.HarnessID != "codex" {
		return Launch{}, domain.Fail("pin_unavailable", "The requested harness is not supported by this execution contract.")
	}
	if version.Pins.AccountID != "" || version.Pins.PaymentPath == "paid_api" {
		return Launch{}, domain.Fail("pin_unavailable", "Native Codex cannot guarantee the requested account or payment path. Remove that explicit pin to use its existing configuration.")
	}
	payload := struct {
		Original   string                `json:"originalInstructions"`
		Guidance   string                `json:"guidance"`
		Acceptance domain.TaskAcceptance `json:"acceptance"`
	}{original, version.GuidanceText, version.Acceptance}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Launch{}, domain.Invalid("valid task instructions")
	}
	prompt := append([]byte("Complete the programming task described by this JSON object. Preserve the complete original instructions and apply its guidance and acceptance requirements. Work only on the requested task in the supplied managed workspace. Return a useful final response; do not publish externally unless explicitly authorized. Council retains your output as candidate evidence and verifies it separately.\n"), encoded...)
	if len(prompt) > domain.MaxPromptBytes {
		return Launch{}, domain.Fail("input_too_large", "The assembled execution prompt exceeds 256 KiB. Reduce the instructions explicitly; stored input has not been truncated.")
	}
	if resolve == nil {
		resolve = ResolveCodex
	}
	executable, err := resolve(ctx, version.Pins)
	if err != nil {
		return Launch{}, err
	}
	args := []string{"exec", "--json", "--ephemeral", "--sandbox", "danger-full-access", "--config", "approval_policy=\"never\"", "--output-last-message", responsePath}
	if version.Pins.Model != "" {
		args = append(args, "--model", version.Pins.Model)
	}
	args = append(args, "-")
	observed := UnknownObservations(mode)
	configured := "danger-full-access / never"
	observed.Permissions.ConfiguredMode = &configured
	return Launch{Executable: executable, Args: args, Prompt: prompt, Observations: observed}, nil
}

// CodexParser is a bounded JSONL decoder. Raw stdout must be retained before Write.
// No item text is interpreted as a check, permission, routing or orchestration command.
type CodexParser struct {
	mu          sync.Mutex
	line        []byte
	result      ProtocolResult
	pins        domain.ExecutionPins
	callback    func(Event) error
	err         error
	finished    bool
	failed      chan struct{}
	failureOnce sync.Once
}

func NewCodexParser(pins domain.ExecutionPins, callback func(Event) error) *CodexParser {
	observed := UnknownObservations("autonomous")
	configured := "danger-full-access / never"
	observed.Permissions.ConfiguredMode = &configured
	return &CodexParser{pins: pins, callback: callback, failed: make(chan struct{}), result: ProtocolResult{Observations: observed}}
}

func (p *CodexParser) fail(code, message string) error {
	p.observeFailure(code, message)
	if p.err == nil {
		p.err = errors.New(message)
	}
	return p.err
}

func (p *CodexParser) Failed() <-chan struct{} { return p.failed }

func (p *CodexParser) observeFailure(code, message string) {
	if p.result.Failure == nil {
		p.result.Failure = &domain.PrimaryFailure{Code: code, Message: message, ObservedAt: domain.Now()}
	}
	p.result.Succeeded = false
	p.failureOnce.Do(func() { close(p.failed) })
}

func (p *CodexParser) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return 0, p.err
	}
	if p.finished {
		return 0, errors.New("Protocol stream is closed.")
	}
	consumed := 0
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		n := len(b)
		if i >= 0 {
			n = i
		}
		if len(p.line)+n > 1<<20 {
			return consumed, p.fail("protocol_limit", "The native JSONL line exceeded 1 MiB. Raw bytes already obtained remain retained.")
		}
		p.line = append(p.line, b[:n]...)
		consumed += n
		b = b[n:]
		if i < 0 {
			break
		}
		consumed++
		b = b[1:]
		if err := p.parse(p.line); err != nil {
			return consumed, err
		}
		p.line = p.line[:0]
	}
	return consumed, nil
}

func (p *CodexParser) parse(line []byte) error {
	if !utf8.Valid(line) {
		return p.fail("protocol_invalid", "Native output contains invalid UTF-8.")
	}
	var obj struct {
		Type     string                    `json:"type"`
		ThreadID string                    `json:"thread_id"`
		Model    string                    `json:"model"`
		Usage    map[string]jsontext.Value `json:"usage"`
	}
	if len(bytes.TrimSpace(line)) == 0 || json.Unmarshal(line, &obj) != nil || obj.Type == "" {
		return p.fail("protocol_invalid", "Native output is not supported JSONL. Check the retained raw output and native CLI compatibility.")
	}
	if len(obj.Type) > 128 || len(obj.ThreadID) > 4096 || len(obj.Model) > 4096 {
		return p.fail("protocol_invalid", "Native protocol metadata exceeds its supported bound.")
	}
	if p.result.Terminal && (obj.Type == "turn.completed" || obj.Type == "turn.failed") {
		return p.fail("protocol_invalid", "Native output contains contradictory or repeated terminal events.")
	}
	source := "native_codex_jsonl"
	at := domain.Now()
	if obj.Type == "thread.started" && obj.ThreadID != "" {
		p.result.Observations.NativeSession = domain.ObservedFact{Status: "observed", Value: &obj.ThreadID, Source: &source, ObservedAt: &at}
	}
	// A model field is recorded only when emitted by a recognized native lifecycle event.
	if (obj.Type == "thread.started" || obj.Type == "turn.started") && obj.Model != "" {
		p.result.Observations.Model = domain.ObservedFact{Status: "observed", Value: &obj.Model, Source: &source, ObservedAt: &at}
		if p.pins.Model != "" && obj.Model != p.pins.Model {
			return p.fail("pin_mismatch", "The native-reported model contradicts the requested exact model pin.")
		}
	}
	if obj.Type == "turn.failed" || obj.Type == "error" {
		p.result.Terminal = obj.Type == "turn.failed"
		p.observeFailure("native_failure", "Native Codex reported failure. Inspect retained output for details.")
	}
	if obj.Type == "turn.completed" {
		p.result.Terminal = true
		p.result.Succeeded = p.result.Failure == nil
		for key, n := range obj.Usage {
			v := string(n)
			parsed, err := strconv.ParseInt(v, 10, 64)
			if err != nil || parsed < 0 || strconv.FormatInt(parsed, 10) != v {
				return p.fail("protocol_invalid", "Native usage must be an exact nonnegative integer.")
			}
			switch key {
			case "input_tokens":
				p.result.Observations.Usage.InputTokens = &v
			case "output_tokens":
				p.result.Observations.Usage.OutputTokens = &v
			case "cached_input_tokens":
				p.result.Observations.Usage.CachedInputTokens = &v
			}
		}
		u := &p.result.Observations.Usage
		if u.InputTokens != nil || u.OutputTokens != nil || u.CachedInputTokens != nil {
			u.Source = &source
			u.ObservedAt = &at
			u.Completeness = "reported_fields"
		}
	}
	if p.callback != nil {
		meta, _ := json.Marshal(map[string]string{"type": obj.Type, "adapterContract": CodexContract})
		observed := p.result.Observations
		if err := p.callback(Event{Kind: "native_event", Metadata: meta, Observations: &observed, Failure: p.result.Failure}); err != nil {
			return p.fail("evidence_failed", "Native observation could not be retained durably.")
		}
	}
	return p.err
}

func (p *CodexParser) Finish() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finished = true
	if p.err != nil {
		return p.err
	}
	if len(p.line) != 0 {
		return p.fail("protocol_invalid", "Native JSONL ended with an incomplete line.")
	}
	return nil
}

func (p *CodexParser) Result() ProtocolResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.result
	if r.Failure != nil {
		f := *r.Failure
		r.Failure = &f
	}
	return r
}

func validNativeString(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
