package engine

import (
	"context"
	json "encoding/json/v2"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func TestRoutingEligibilityMutationCannotCommitBetweenGuardAndPreparation(t *testing.T) {
	e, project, _ := testEngine(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		close(entered)
		select {
		case <-release:
			return process.Prepare(ctx, spec)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	a := admit(t, e, project, "response")
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("fixture did not reach controlled native preparation")
	}
	body, _ := json.Marshal(domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: boolPointer(false)})
	done := make(chan domain.Result, 1)
	go func() { done <- e.MutateRouting(context.Background(), "routing.context.update", "codex-native", body) }()
	select {
	case result := <-done:
		t.Fatal("eligibility changed inside the ordered preparation interval", result)
	case <-time.After(100 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	select {
	case result := <-done:
		if result.Envelope.Error != nil {
			t.Fatal(result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the completed preparation kept an eligibility mutation blocked")
	}
	current := settled(t, e, a.Attempt.ID)
	if current.Cleanup.State != "verified" {
		t.Fatal("the ordered launch did not retain owned cleanup", current)
	}
}

func TestRoutingStopRemainsResponsiveWhilePreparationGateIsHeld(t *testing.T) {
	e, project, _ := testEngine(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	e.config.Prepare = func(ctx context.Context, _ process.NativeSpec) (*process.Owned, error) {
		close(entered)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return nil, context.Canceled
		}
	}
	a := admit(t, e, project, "response")
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("fixture did not reach controlled preparation")
	}
	done := make(chan domain.Result, 1)
	go func() { done <- e.Stop(context.Background(), a.Attempt.ID, domain.NewID()) }()
	select {
	case result := <-done:
		if result.Envelope.Error != nil || result.Envelope.Receipt == nil {
			t.Fatal("Stop was not durably acknowledged", result)
		}
	case <-time.After(2 * time.Second):
		once.Do(func() { close(release) })
		t.Fatal("preparation gate prevented timely Stop acceptance")
	}
	current := settled(t, e, a.Attempt.ID)
	if current.Outcome == nil || *current.Outcome != "cancelled" || current.StartedAt != nil || current.Cleanup.State != "verified" {
		t.Fatal("Stop did not prevent worker GO during preparation", current)
	}
}

func boolPointer(value bool) *bool { return &value }
