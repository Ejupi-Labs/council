package workflow

import (
	json "encoding/json/v2"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func reviewFixture() (domain.ReviewReport, domain.ConditionRegistry, []domain.ReviewInspection) {
	report := domain.ReviewReport{SchemaVersion: domain.ReviewContract, Subject: domain.ReviewSubject{Kind: "candidate", ID: strings.Repeat("a", 32), SHA256: strings.Repeat("b", 64)}, ConditionsSHA256: strings.Repeat("c", 64), Outcome: "approved", InspectedEvidence: []domain.ReviewInspection{{Ref: "candidate-manifest", Scope: "whole_candidate", Paths: []string{}}}, Findings: []domain.ReviewFinding{}, Gaps: []domain.ReviewGap{}, ResolvedFindingIDs: []string{}}
	access := append([]domain.ReviewInspection{}, report.InspectedEvidence...)
	for i := range access {
		access[i].Paths = append([]string{}, access[i].Paths...)
	}
	return report, domain.ConditionRegistry{{Key: "task-instructions", Kind: "instructions", Source: "task", EvidenceRefs: []string{"original"}}}, access
}

func TestReviewRejectsWrongSubjectInsteadOfApprovingAnotherCandidate(t *testing.T) {
	report, registry, supplied := reviewFixture()
	expected := report.Subject
	report.Subject.ID = strings.Repeat("d", 32)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if _, e := ValidateReview(raw, expected, report.ConditionsSHA256, registry, supplied, nil); e == nil {
		t.Fatal("review for a different candidate was accepted")
	}
}

func TestPlanPlainResponsePausesWithoutIncidentalConventionOrModelReview(t *testing.T) {
	decision := Plan(PlanInput{HasSubject: true, ResponseOnly: true, Authority: domain.WorkflowAuthority{Review: "auto", Checks: []domain.CheckDefinition{{ID: strings.Repeat("a", 32), Key: "go-tests", Kind: "command", Source: "recognized_convention", Required: true}}}})
	if decision.Owner != "user" || decision.Action != "response_acceptance_pending" || decision.Role != "" {
		t.Fatalf("plain response added model/check work: %+v", decision)
	}
}

func TestReviewAcceptsExactClosedReportAndRejectsUnsupportedAttribution(t *testing.T) {
	report, registry, supplied := reviewFixture()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if _, e := ValidateReview(raw, report.Subject, report.ConditionsSHA256, registry, supplied, nil); e != nil {
		t.Fatal("valid exact report rejected", e)
	}
	for name, change := range map[string]func(*domain.ReviewReport){
		"conditions":  func(r *domain.ReviewReport) { r.ConditionsSHA256 = strings.Repeat("d", 64) },
		"uninspected": func(r *domain.ReviewReport) { r.InspectedEvidence = []domain.ReviewInspection{} },
		"unknown ref": func(r *domain.ReviewReport) { r.InspectedEvidence[0].Ref = "not-supplied" },
		"retroactive subset": func(r *domain.ReviewReport) {
			r.InspectedEvidence[0].Scope = "declared_subset"
			r.InspectedEvidence[0].Paths = []string{"not-captured.txt"}
		},
		"approval blocker": func(r *domain.ReviewReport) {
			r.Findings = []domain.ReviewFinding{{Key: "blocker", Severity: "blocking", ConditionKeys: []string{"task-instructions"}, Locations: []domain.ReviewLocation{}, Summary: "Actual defect", EvidenceRefs: []string{"candidate-manifest"}}}
		},
		"model resolution": func(r *domain.ReviewReport) { r.ResolvedFindingIDs = []string{strings.Repeat("f", 32)} },
	} {
		t.Run(name, func(t *testing.T) {
			value, conditions, access := reviewFixture()
			change(&value)
			raw, _ := json.Marshal(value)
			if _, e := ValidateReview(raw, report.Subject, report.ConditionsSHA256, conditions, access, nil); e == nil {
				t.Fatal("invalid report qualified")
			}
		})
	}
	missing := strings.Replace(string(raw), `,"paths":[]`, "", 1)
	if _, e := ValidateReview([]byte(missing), report.Subject, report.ConditionsSHA256, registry, supplied, nil); e == nil {
		t.Fatal("missing required nested native field accepted")
	}
	if _, e := ValidateReview(append(raw, []byte("\n{}")...), report.Subject, report.ConditionsSHA256, registry, supplied, nil); e == nil {
		t.Fatal("trailing second review object accepted")
	}
}

func TestReviewFindingsRequireActualSuppliedPathAndKnownCondition(t *testing.T) {
	report, registry, supplied := reviewFixture()
	report.Outcome = "changes_required"
	report.Findings = []domain.ReviewFinding{{Key: "actual-defect", Severity: "blocking", ConditionKeys: []string{"task-instructions"}, Locations: []domain.ReviewLocation{{Path: "src/controlled.go", StartLine: 1, EndLine: 2}}, Summary: "Observed defect in the supplied candidate", EvidenceRefs: []string{"candidate-manifest"}}}
	raw, _ := json.Marshal(report)
	if _, e := ValidateReview(raw, report.Subject, report.ConditionsSHA256, registry, supplied, nil); e == nil {
		t.Fatal("invented path accepted without actual manifest authority")
	}
	if _, e := ValidateReview(raw, report.Subject, report.ConditionsSHA256, registry, supplied, nil, []string{"src/controlled.go"}); e != nil {
		t.Fatal("actual supplied path rejected", e)
	}
}
