package domain

import (
	"strings"
	"testing"

	json "encoding/json/v2"
)

func TestWorkflowStrictDeclarationsRejectAmbiguousPresence(t *testing.T) {
	for name, option := range map[string]string{
		"empty mode":                `{"mode":""}`,
		"null workflow member":      `{"delivery":null}`,
		"empty local mode":          `{"delivery":{"localMode":""}}`,
		"missing command arguments": `{"checks":[{"key":"tests","executable":"go"}]}`,
		"empty cwd":                 `{"checks":[{"key":"tests","executable":"go","arguments":[],"relativeWorkingDirectory":""}]}`,
		"null false option":         `{"checks":[{"key":"tests","executable":"go","arguments":[],"required":null}]}`,
		"unknown trusted field":     `{"checks":[{"key":"tests","executable":"go","arguments":[],"physicalIdentity":"client-proof"}]}`,
		"missing scope paths":       `{"checks":[{"key":"tests","executable":"go","arguments":[],"scope":{"kind":"whole_candidate"}}]}`,
		"shell wrapper":             `{"checks":[{"key":"tests","executable":"C:\\tools\\npm.cmd","arguments":[]}]}`,
		"empty success codes":       `{"checks":[{"key":"tests","executable":"go","arguments":[],"successExitCodes":[]}]}`,
		"unsafe numeric code":       `{"checks":[{"key":"tests","executable":"go","arguments":[],"successExitCodes":[9007199254740993]}]}`,
		"reserved key":              `{"checks":[{"key":"task-instructions","executable":"go","arguments":[]}]}`,
		"author only scope":         `{"mode":"author_only","delivery":{"localMode":"review_only"}}`,
		"duplicate member":          `{"mode":"auto","mode":"author_only"}`,
	} {
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"requestId":"11111111111111111111111111111111","projectId":"22222222222222222222222222222222","originalText":"Retain the exact task.","workflow":` + option + `}`)
			var in StartTaskInput
			if e := StrictDecode(body, &in); e == nil && ValidateStartTask(in) == nil {
				t.Fatalf("accepted ambiguous declaration: %s", option)
			}
		})
	}
}

func TestWorkflowOmissionEmptyFalseAndZeroRemainDistinct(t *testing.T) {
	var omitted, explicit WorkflowInput
	if e := StrictDecode([]byte(`{}`), &omitted); e != nil {
		t.Fatal(e)
	}
	if e := StrictDecode([]byte(`{"checks":[],"maxCorrectionCycles":0}`), &explicit); e != nil {
		t.Fatal(e)
	}
	if explicit.Checks == nil || explicit.MaxCorrectionCycles == nil || *explicit.MaxCorrectionCycles != 0 {
		t.Fatal("explicit authority restriction disappeared")
	}
	a, _ := json.Marshal(omitted)
	b, _ := json.Marshal(explicit)
	if string(a) == string(b) || !strings.Contains(string(b), `"checks":[]`) || !strings.Contains(string(b), `"maxCorrectionCycles":0`) {
		t.Fatal("receipt shape collapsed", string(a), string(b))
	}
	checks, e := StrictDecodeCheckDeclarations([]byte(`[{"key":"tests","executable":"go","arguments":[],"required":false,"successExitCodes":[0]}]`))
	if e != nil || len(checks) != 1 || checks[0].Required == nil || *checks[0].Required || checks[0].SuccessExitCodes == nil {
		t.Fatal("explicit false/codes lost", checks, e)
	}
}

func TestWorkflowExternalDeclarationCannotSmuggleCheckOrProofFields(t *testing.T) {
	for _, command := range []string{
		`{"executable":"/fixture/publisher","arguments":[],"successExitCodes":[0]}`,
		`{"executable":"/fixture/publisher","arguments":[],"physicalIdentity":"proof"}`,
		`{"executable":"/fixture/publisher"}`,
	} {
		var in WorkflowDeliveryInput
		body := []byte(`{"externalAction":{"action":"publish-preview","destination":"fixture","acceptanceBasis":"command_completion","command":` + command + `}}`)
		if e := StrictDecode(body, &in); e == nil && ValidateWorkflowDeliveryInput(in) == nil {
			t.Fatal("accepted undeclared command shape", command)
		}
	}
	var valid WorkflowDeliveryInput
	if e := StrictDecode([]byte(`{"externalAction":{"action":"publish-preview","destination":"fixture","acceptanceBasis":"command_completion","command":{"executable":"/fixture/publisher","arguments":[]}}}`), &valid); e != nil {
		t.Fatal(e)
	}
	if e := ValidateWorkflowDeliveryInput(valid); e != nil {
		t.Fatal(e)
	}
	valid.ExternalAction.AcceptanceBasis = "remote_deployed"
	if e := ValidateWorkflowDeliveryInput(valid); e == nil || e.Code != "external_postcondition_unsupported" {
		t.Fatal("invented remote qualification", e)
	}
}

func TestWorkflowNativeReviewRequiresEveryNestedField(t *testing.T) {
	valid := `{"schemaVersion":"005-review-v1","subject":{"kind":"candidate","id":"11111111111111111111111111111111","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"conditionsSHA256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","outcome":"approved","inspectedEvidence":[{"ref":"manifest","scope":"whole_candidate","paths":[]}],"findings":[],"gaps":[],"resolvedFindingIds":[]}`
	var report ReviewReport
	if e := StrictDecode([]byte(valid), &report); e != nil {
		t.Fatal(e)
	}
	for name, raw := range map[string]string{
		"missing root array":       strings.Replace(valid, `,"gaps":[]`, "", 1),
		"missing inspection paths": strings.Replace(valid, `,"paths":[]`, "", 1),
		"null array":               strings.Replace(valid, `"findings":[]`, `"findings":null`, 1),
		"missing location end":     strings.Replace(valid, `"findings":[]`, `"findings":[{"key":"defect","severity":"blocking","conditionKeys":["task-instructions"],"locations":[{"path":"file.txt","startLine":1}],"summary":"Fix the defect.","evidenceRefs":["manifest"]}]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			var in ReviewReport
			if e := StrictDecode([]byte(raw), &in); e == nil {
				t.Fatal("partial native object accepted")
			}
		})
	}
}

func TestWorkflowControlExactRevisionAndSelectorBoundaries(t *testing.T) {
	max := WorkflowAdvanceInput{RequestID: strings.Repeat("1", 32), ExpectedRevision: "9223372036854775807", WorkflowRevision: "9007199254740993", TaskVersion: "1"}
	if e := ValidateWorkflowAdvanceInput(max); e != nil {
		t.Fatal(e)
	}
	max.ExpectedRevision = "9223372036854775808"
	if e := ValidateWorkflowAdvanceInput(max); e == nil {
		t.Fatal("revision overflow accepted")
	}
	if e := ValidateWorkflowSelector(WorkflowSelector{WorkflowID: strings.Repeat("2", 32), TaskVersion: "1"}); e == nil {
		t.Fatal("two historical selectors accepted")
	}
	if e := ValidateWorkflowSelector(WorkflowSelector{TaskVersion: "9007199254740993"}); e != nil {
		t.Fatal(e)
	}
}

func TestWorkflowExistingAccountProviderClearRetainsWirePresence(t *testing.T) {
	empty := ""
	b, err := json.Marshal(AccountUpdateInput{RequestID: strings.Repeat("1", 32), ExpectedRevision: "1", ProviderLabel: &empty})
	if err != nil || !strings.Contains(string(b), `"providerLabel":""`) {
		t.Fatal("explicit account label clear disappeared", string(b), err)
	}
	b, err = json.Marshal(AccountUpdateInput{RequestID: strings.Repeat("1", 32), ExpectedRevision: "1"})
	if err != nil || strings.Contains(string(b), `"providerLabel"`) {
		t.Fatal("omitted account label became a clear", string(b), err)
	}
}

func TestWorkflowOptionalExecutionTagsRemainAbsentForLegacyRecords(t *testing.T) {
	b, err := json.Marshal(Attempt{ID: strings.Repeat("1", 32)})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"workflowId", "stepId", "role", "executorKind", "inputCandidateId", "trigger"} {
		if strings.Contains(string(b), `"`+key+`"`) {
			t.Fatal("historical shape gained a default", key)
		}
	}
}
