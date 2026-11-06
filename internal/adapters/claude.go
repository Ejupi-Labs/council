package adapters

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const nativeLineLimit = 1 << 20

// ClaudeParser accepts the declared one-prompt native profile. It never extracts
// response text from assistant prose or treats native prose as orchestration.
type ClaudeParser struct {
	reviewProfile    bool
	mu               sync.Mutex
	line             []byte
	result           ProtocolResult
	stdoutID         string
	callback         func(Event) error
	err              error
	finished         bool
	terminalOK       bool
	fake             bool
	session          string
	mainModel        string
	modelAmbiguous   bool
	optionalOmission bool
	failed           chan struct{}
	failureOnce      sync.Once
}

func NewClaudeParser(_ domain.ExecutionPins, stdoutArtifactID string, emit func(Event) error) *ClaudeParser {
	observed := UnknownObservations("autonomous")
	configured := "bypassPermissions / permission-prompts none"
	observed.Permissions.ConfiguredMode = &configured
	return &ClaudeParser{stdoutID: stdoutArtifactID, callback: emit, failed: make(chan struct{}), result: ProtocolResult{Observations: observed}}
}

func (p *ClaudeParser) Failed() <-chan struct{} { return p.failed }

func (p *ClaudeParser) observeFailure(code, message string) {
	if p.result.Failure == nil {
		p.result.Failure = &domain.PrimaryFailure{Code: code, Message: message, ObservedAt: domain.Now()}
	}
	p.result.Succeeded = false
	p.failureOnce.Do(func() { close(p.failed) })
}

func (p *ClaudeParser) fail(code, message string) error {
	p.observeFailure(code, message)
	if p.err == nil {
		p.err = errors.New(message)
	}
	return p.err
}

func (p *ClaudeParser) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return 0, p.err
	}
	if p.finished {
		return 0, errors.New("Protocol stream is closed.")
	}
	consumed := 0
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		length := len(data)
		if index >= 0 {
			length = index
		}
		if len(p.line)+length > nativeLineLimit {
			return consumed, p.fail("protocol_limit", "The native JSONL line exceeded 1 MiB. Raw bytes already obtained remain retained.")
		}
		p.line = append(p.line, data[:length]...)
		consumed += length
		data = data[length:]
		if index < 0 {
			break
		}
		consumed++
		data = data[1:]
		if err := p.parse(p.line); err != nil {
			return consumed, err
		}
		p.line = p.line[:0]
	}
	return consumed, nil
}

func rawObject(value jsontext.Value) (map[string]jsontext.Value, bool) {
	if len(value) == 0 || value.Kind() != '{' {
		return nil, false
	}
	var object map[string]jsontext.Value
	err := json.Unmarshal(value, &object)
	return object, err == nil && object != nil
}

func nativeText(value jsontext.Value) (string, bool) {
	if len(value) == 0 || value.Kind() != '"' {
		return "", false
	}
	var text string
	err := json.Unmarshal(value, &text)
	return text, err == nil
}

func nativeIdentity(value jsontext.Value) (string, bool) {
	text, ok := nativeText(value)
	return text, ok && text != "" && len(text) <= domain.MaxNativeIdentityBytes && validNativeString(text)
}

func observedFact(value, source string) domain.ObservedFact {
	at := domain.Now()
	return domain.ObservedFact{Status: "observed", Value: &value, Source: &source, ObservedAt: &at}
}

func (p *ClaudeParser) recordSession(object map[string]jsontext.Value, required bool, source string) error {
	value, present := object["session_id"]
	if !present && !required {
		return nil
	}
	id, ok := nativeIdentity(value)
	if !ok || (p.session != "" && p.session != id) {
		return p.fail("protocol_invalid", "The native main-session identity is missing, unsupported or conflicting.")
	}
	p.session = id
	if !p.fake && source != "" {
		p.result.Observations.NativeSession = observedFact(id, source)
	}
	return nil
}

func (p *ClaudeParser) parse(line []byte) error {
	if !utf8.Valid(line) {
		return p.fail("protocol_invalid", "Native output contains invalid UTF-8.")
	}
	var object map[string]jsontext.Value
	if len(bytes.TrimSpace(line)) == 0 || json.Unmarshal(line, &object) != nil || object == nil {
		return p.fail("protocol_invalid", "Native output is not supported JSONL. Inspect retained output and native CLI compatibility.")
	}
	kind, ok := nativeText(object["type"])
	if !ok || kind == "" || len(kind) > 128 || !validNativeString(kind) {
		return p.fail("protocol_invalid", "Native output has an unsupported message type.")
	}
	if object["council_fixture"].Kind() == 't' {
		p.fake = true
		p.result.Observations = UnknownObservations("autonomous")
		configured := "bypassPermissions / permission-prompts none"
		p.result.Observations.Permissions.ConfiguredMode = &configured
	}
	recognized := kind == "system" || kind == "assistant" || kind == "user" || kind == "result" || kind == "tool_progress" || kind == "rate_limit_event" || kind == "stream_event" || kind == "auth_status"
	parent, attributed := object["parent_tool_use_id"]
	main := attributed && parent.Kind() == 'n'
	subagent := false
	if attributed && !main {
		id, valid := nativeIdentity(parent)
		subagent = valid && id != ""
		if recognized && !subagent {
			return p.fail("protocol_invalid", "A recognized native message has an invalid parent identity.")
		}
	}
	// Known main-message kinds, including trailing observations, cannot silently
	// switch conversations. Absent assistant parent attribution stays ambiguous;
	// unknown future message kinds remain raw evidence and establish no identity.
	if recognized && !subagent {
		if err := p.recordSession(object, false, ""); err != nil {
			return err
		}
	}
	switch kind {
	case "system":
		subtype, _ := nativeText(object["subtype"])
		if subtype == "init" && !subagent {
			if err := p.recordSession(object, true, "claude.system.init.session_id"); err != nil {
				return err
			}
			p.readInit(object)
		}
		if subtype == "error" || subtype == "api_error" || subtype == "model_refusal_no_fallback" {
			p.observeFailure("native_failure", "Native Claude reported an authoritative error. Inspect retained output for details.")
		}
	case "assistant":
		if value, present := object["error"]; present && value.Kind() != 'n' {
			text, valid := nativeText(value)
			if !valid || text != "" {
				p.observeFailure("native_failure", "Native Claude reported an assistant error. Inspect retained output for details.")
			}
		}
		if main {
			if err := p.recordSession(object, false, ""); err != nil {
				return err
			}
		}
		p.readAssistant(object, main, subagent)
	case "result":
		if !subagent {
			if err := p.readResult(object); err != nil {
				return err
			}
		}
	case "error":
		p.observeFailure("native_failure", "Native Claude reported an error. Inspect retained output for details.")
	}
	return p.emit(kind)
}

func (p *ClaudeParser) readInit(object map[string]jsontext.Value) {
	if p.fake {
		return
	}
	if value, present := object["model"]; present {
		fact := domain.ObservedFact{Status: "unknown"}
		if model, ok := nativeIdentity(value); ok {
			fact = observedFact(model, "claude.system.init.model")
		} else {
			p.optionalOmission = true
		}
		p.result.Observations.ConfiguredModel = &fact
	}
	if value, present := object["claude_code_version"]; present {
		fact := domain.ObservedFact{Status: "unknown"}
		if version, ok := nativeIdentity(value); ok {
			fact = observedFact(version, "claude.system.init.claude_code_version")
			if knownOldClaudeVersion(version) {
				p.observeFailure("native_incompatible", "Native Claude reported a version below the declared 2.1.259 profile. Upgrade using vendor instructions.")
			}
		} else {
			p.optionalOmission = true
		}
		p.result.Observations.NativeVersion = &fact
	}
	if value, present := object["permissionMode"]; present {
		if mode, ok := nativeIdentity(value); ok {
			p.result.Observations.Permissions.NativeReportedMode = &mode
			if mode != "bypassPermissions" {
				p.observeFailure("permission_unavailable", "Native Claude did not report the requested bypass permission mode. Inspect native or managed restrictions.")
			}
		} else {
			p.optionalOmission = true
		}
	}
}

func knownOldClaudeVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	var parsed [3]int64
	for i := range parts {
		n, err := strconv.ParseInt(parts[i], 10, 32)
		if err != nil || n < 0 || strconv.FormatInt(n, 10) != parts[i] {
			return false
		}
		parsed[i] = n
	}
	return parsed[0] < 2 || parsed[0] == 2 && (parsed[1] < 1 || parsed[1] == 1 && parsed[2] < 259)
}

func (p *ClaudeParser) readAssistant(object map[string]jsontext.Value, main, subagent bool) {
	if p.fake || subagent {
		return
	}
	message, valid := rawObject(object["message"])
	model, modelOK := nativeIdentity(message["model"])
	if !main || !valid || !modelOK {
		p.modelAmbiguous = true
		p.optionalOmission = true
	} else if p.mainModel == "" {
		p.mainModel = model
	} else if p.mainModel != model {
		p.modelAmbiguous = true
	}
	if p.modelAmbiguous {
		p.result.Observations.Model = domain.ObservedFact{Status: "unknown"}
	} else {
		p.result.Observations.Model = observedFact(p.mainModel, "claude.assistant.message.model")
	}
}

func (p *ClaudeParser) readResult(object map[string]jsontext.Value) error {
	if p.result.Terminal {
		return p.fail("protocol_invalid", "Native Claude emitted repeated or conflicting main results; this one-shot profile does not select the first or last result.")
	}
	if err := p.recordSession(object, true, "claude.result.session_id"); err != nil {
		return err
	}
	subtype, valid := nativeText(object["subtype"])
	var isError bool
	flag, present := object["is_error"]
	if !valid || subtype == "" || len(subtype) > 128 || !present || (flag.Kind() != 't' && flag.Kind() != 'f') || json.Unmarshal(flag, &isError) != nil {
		return p.fail("protocol_invalid", "Native Claude result lacks its mandatory typed terminal fields.")
	}
	p.result.Terminal = true
	stopReason, _ := nativeText(object["stop_reason"])
	if subtype != "success" || isError || stopReason == "refusal" || stopReason == "tool_deferred_unavailable" {
		p.observeFailure("native_failure", "Native Claude reported an unsuccessful terminal result. Inspect retained output for details.")
	}
	if subtype == "success" && p.reviewProfile {
		structured, present := object["structured_output"]
		if !present || structured.Kind() != '{' {
			return p.fail("protocol_invalid", "Native Claude review success has no supported structured output object.")
		}
		if p.result.Failure == nil {
			p.result.StructuredOutput = bytes.Clone(structured)
			p.terminalOK = true
		}
	} else if subtype == "success" {
		response, ok := nativeText(object["result"])
		if !ok {
			return p.fail("protocol_invalid", "Native Claude success result has no supported final-response string.")
		}
		if p.result.Failure == nil {
			p.result.FinalResponse = &response
			p.terminalOK = true
		}
	}
	if !p.fake {
		p.readUsage(object["usage"])
		if value, present := object["modelUsage"]; present {
			p.readModels(value)
		}
	}
	return nil
}

func nativeCount(value jsontext.Value) (*string, bool) {
	if len(value) == 0 {
		return nil, true
	}
	if value.Kind() != '0' {
		return nil, false
	}
	text := string(value)
	if _, err := domain.ParseCount(text); err != nil {
		return nil, false
	}
	return &text, true
}

func (p *ClaudeParser) readUsage(value jsontext.Value) {
	usage := &p.result.Observations.Usage
	usage.Scope = "top_level"
	if len(value) == 0 {
		return
	}
	object, ok := rawObject(value)
	if !ok {
		p.optionalOmission = true
		return
	}
	valid := true
	for _, field := range []struct {
		key string
		to  **string
	}{{"input_tokens", &usage.InputTokens}, {"output_tokens", &usage.OutputTokens}, {"cache_read_input_tokens", &usage.CachedInputTokens}, {"cache_creation_input_tokens", &usage.CacheCreationInputTokens}} {
		count, ok := nativeCount(object[field.key])
		*field.to = count
		valid = valid && ok
	}
	source, at := "claude.result.usage", domain.Now()
	usage.Source, usage.ObservedAt = &source, &at
	usage.Completeness = "complete"
	if !valid {
		usage.Completeness = "partial"
		p.optionalOmission = true
	} else if usage.InputTokens == nil && usage.OutputTokens == nil && usage.CachedInputTokens == nil && usage.CacheCreationInputTokens == nil {
		usage.Completeness = "unknown"
	}
}

func (p *ClaudeParser) readModels(value jsontext.Value) {
	preview := &domain.NativeModelsObservation{Scope: "whole_tree", Source: "claude.result.modelUsage", ObservedAt: domain.Now(), Completeness: "unknown", EvidenceArtifactIDs: []string{p.stdoutID}, Models: []domain.NativeModelUsage{}}
	p.result.Observations.NativeModels = preview
	object, ok := rawObject(value)
	if !ok {
		p.optionalOmission = true
		return
	}
	preview.Completeness = "complete"
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	omitted := int64(0)
	for _, model := range keys {
		fields, valid := rawObject(object[model])
		if model == "" || !validNativeString(model) || len(model) > domain.MaxNativeIdentityBytes || !valid || len(preview.Models) >= domain.MaxNativeModels {
			omitted++
			preview.Completeness = "partial"
			continue
		}
		entry := domain.NativeModelUsage{Model: model}
		for _, field := range []struct {
			key string
			to  **string
		}{{"inputTokens", &entry.InputTokens}, {"outputTokens", &entry.OutputTokens}, {"cacheReadInputTokens", &entry.CachedInputTokens}, {"cacheCreationInputTokens", &entry.CacheCreationInputTokens}} {
			count, ok := nativeCount(fields[field.key])
			*field.to = count
			if !ok {
				preview.Completeness = "partial"
			}
		}
		if cost, present := fields["costUSD"]; present {
			if cost.Kind() == '0' && len(cost) <= 64 && cost[0] != '-' {
				text := string(cost) // Retain a mathematical JSON decimal; never float64.
				entry.CostEstimateUSD = &text
			} else {
				preview.Completeness = "partial"
			}
		}
		preview.Models = append(preview.Models, entry)
	}
	count := strconv.FormatInt(omitted, 10)
	preview.OmittedModelCount = &count
	if preview.Completeness != "complete" {
		p.optionalOmission = true
	}
}

func cloneObservations(observed domain.TargetObservations) domain.TargetObservations {
	encoded, _ := json.Marshal(observed)
	var copy domain.TargetObservations
	_ = json.Unmarshal(encoded, &copy)
	return copy
}

func (p *ClaudeParser) emit(kind string) error {
	var metadata []byte
	for {
		var evidenceIDs []string
		if p.optionalOmission {
			evidenceIDs = []string{p.stdoutID}
		}
		metadata, _ = json.Marshal(struct {
			Type                string   `json:"type"`
			Contract            string   `json:"adapterContract"`
			Omitted             bool     `json:"optionalObservationOmitted,omitempty"`
			EvidenceArtifactIDs []string `json:"evidenceArtifactIds,omitempty"`
		}{kind, ClaudeContract, p.optionalOmission, evidenceIDs})
		encoded, _ := json.Marshal(Event{Kind: "native_event", Metadata: metadata, Observations: &p.result.Observations, Failure: p.result.Failure})
		if len(encoded) <= domain.MaxEventBytes {
			break
		}
		preview := p.result.Observations.NativeModels
		if preview == nil || len(preview.Models) == 0 {
			return p.fail("protocol_limit", "Required native observation metadata exceeds 16 KiB; retained raw output remains available.")
		}
		preview.Models = preview.Models[:len(preview.Models)-1]
		preview.Completeness = "partial"
		omitted, _ := strconv.ParseInt(*preview.OmittedModelCount, 10, 64)
		count := strconv.FormatInt(omitted+1, 10)
		preview.OmittedModelCount = &count
		p.optionalOmission = true
	}
	if p.callback != nil {
		observed := cloneObservations(p.result.Observations)
		var failure *domain.PrimaryFailure
		if p.result.Failure != nil {
			copy := *p.result.Failure
			failure = &copy
		}
		if err := p.callback(Event{Kind: "native_event", Metadata: metadata, Observations: &observed, Failure: failure}); err != nil {
			return p.fail("evidence_failed", "Native observation could not be retained durably.")
		}
	}
	return p.err
}

func (p *ClaudeParser) Finish() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finished = true
	if p.err != nil {
		return p.err
	}
	if len(p.line) != 0 || !p.result.Terminal {
		return p.fail("protocol_invalid", "Native Claude output ended without one complete main terminal result.")
	}
	p.result.Succeeded = p.terminalOK && p.result.Failure == nil
	return nil
}

func (p *ClaudeParser) Result() ProtocolResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := p.result
	result.StructuredOutput = bytes.Clone(result.StructuredOutput)
	result.Observations = cloneObservations(result.Observations)
	if result.Failure != nil {
		copy := *result.Failure
		result.Failure = &copy
	}
	if result.FinalResponse != nil {
		copy := *result.FinalResponse
		result.FinalResponse = &copy
	}
	return result
}
