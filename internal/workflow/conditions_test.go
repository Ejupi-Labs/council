package workflow

import (
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestConditionDigestsPreserveExactBodiesAndCanonicalOrder(t *testing.T) {
	version := domain.TaskVersion{GuidanceText: "\ufeffg 東京\r\n", Acceptance: domain.TaskAcceptance{Deliverable: "candidate", ConditionsText: "exact café\r\n", RequiredEvidence: []string{}}}
	defs := []domain.CheckDefinition{{Key: "z", DefinitionSHA256: "z", Required: true}, {Key: "a", DefinitionSHA256: "a", Required: true}}
	registry, captureErr := Conditions("original\r\n", version, domain.WorkflowInput{}, defs)
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	a, b, err := ConditionDigests("original\r\n", version, registry, defs)
	if err != nil {
		t.Fatal(err)
	}
	defs[0], defs[1] = defs[1], defs[0]
	registry[0], registry[1] = registry[1], registry[0]
	c, d, err := ConditionDigests("original\r\n", version, registry, defs)
	if err != nil || a != c || b != d {
		t.Fatal("equivalent ordering changed canonical digest", err)
	}
	c, _, _ = ConditionDigests("original\n", version, registry, defs)
	if c == a {
		t.Fatal("instruction digest normalized original bytes")
	}
	version.Acceptance.ConditionsText = "exact café\n"
	_, d, _ = ConditionDigests("original\r\n", version, registry, defs)
	if d == b {
		t.Fatal("conditions digest normalized required bytes")
	}
}
