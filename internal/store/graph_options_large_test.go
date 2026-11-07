package store

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphNodeOptionsReadsValidLargeGuidanceWithoutClipping(t *testing.T) {
	s, graph, expected, second := graphVersionOptionsFixture(t)
	// Each field is independently valid and may be retained from separate
	// guidance updates. JSON escaping expands each control byte to six bytes.
	second.GuidanceText = strings.Repeat("\x00", domain.MaxTextBytes)
	second.Acceptance.ConditionsText = strings.Repeat("\x01", domain.MaxTextBytes)
	if invalid := domain.ValidateAdditionalText(second.GuidanceText); invalid != nil {
		t.Fatal(invalid)
	}
	if _, invalid := domain.ResolveAcceptance(&domain.AcceptanceInput{ConditionsText: second.Acceptance.ConditionsText}); invalid != nil {
		t.Fatal(invalid)
	}
	// First retain acceptance alone, then inherit it while adding guidance:
	// each request fits the ordinary 32 MiB body bound; the combined row is larger.
	third := second
	third.Version = "3"
	thirdOptions := *second.GraphOptions
	thirdOptions.TaskVersion = "3"
	third.GraphOptions = &thirdOptions
	second.GuidanceText = ""
	installGraphOptionsVersion(t, s, graph, second)
	installGraphOptionsVersion(t, s, graph, third)
	options, e := s.GraphNodeOptions(context.Background(), expected.GraphID, expected.NodeID, "3")
	if e != nil || !reflect.DeepEqual(options, *third.GraphOptions) {
		t.Fatal("valid large guidance made immutable execution options unreadable", options, e)
	}
	public, e := s.Task(context.Background(), expected.TaskID)
	if e != nil || public.Version.GuidanceText != third.GuidanceText || public.Version.Acceptance.ConditionsText != third.Acceptance.ConditionsText {
		t.Fatal("large version was clipped or normalised", e)
	}
}
