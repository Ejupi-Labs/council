package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strings"
	"testing"
)

func TestSecondaryFailureDetailsSurviveOrderedStopAndSettlement(t *testing.T) {
	for _, stopFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure_first", true: "stop_first"}[stopFirst], func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			project := openFixture(t, s, "secondary")
			owner := domain.NewID()
			result := s.CreateTask(ctx, owner, taskInput(project.ID))
			if result.Envelope.Error != nil {
				t.Fatal(result.Envelope.Error)
			}
			admitted := result.Envelope.Data.(domain.Admission)
			launchFixture(t, s, admitted, owner)
			if stopFirst {
				if stopped := s.StopAttempt(ctx, admitted.Attempt.ID, domain.NewID()); stopped.Envelope.Error != nil {
					t.Fatal(stopped.Envelope.Error)
				}
			}
			if _, err := s.RecordAttempt(ctx, admitted.Attempt.ID, domain.AttemptUpdate{Kind: "failure_observed", Failure: &domain.PrimaryFailure{Code: "native_failed_first", Message: "The native operation failed."}}); err != nil {
				t.Fatal(err)
			}
			if !stopFirst {
				if stopped := s.StopAttempt(ctx, admitted.Attempt.ID, domain.NewID()); stopped.Envelope.Error != nil {
					t.Fatal(stopped.Envelope.Error)
				}
			}
			if _, err := s.RecordAttempt(ctx, admitted.Attempt.ID, domain.AttemptUpdate{Kind: "failure_observed", Failure: &domain.PrimaryFailure{Code: "secondary_flush_fault", Message: "A later evidence checkpoint failed."}}); err != nil {
				t.Fatal(err)
			}
			settled, err := s.SettleAttempt(ctx, admitted.Attempt.ID, domain.Settlement{Outcome: "failed", Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "secondary_capture_fault", Message: "A later candidate capture failed."}})
			if err != nil {
				t.Fatal(err)
			}
			want := "failed"
			if stopFirst {
				want = "cancelled"
			}
			if settled.Outcome == nil || *settled.Outcome != want {
				t.Fatalf("Ordered outcome changed: %+v", settled)
			}
			if !stopFirst && (settled.PrimaryFailure == nil || settled.PrimaryFailure.Code != "native_failed_first") {
				t.Fatal("A secondary fault replaced the primary failure")
			}
			page, err := s.Events(ctx, settled.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			var metadata strings.Builder
			for _, event := range page.Items {
				metadata.Write(event.Metadata)
			}
			for _, code := range []string{"secondary_flush_fault", "secondary_capture_fault"} {
				if !strings.Contains(metadata.String(), code) {
					t.Fatalf("Lost secondary fault %s", code)
				}
			}
		})
	}
}
