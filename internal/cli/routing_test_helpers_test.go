package cli

import (
	"context"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func routingReadAttempt(t *testing.T, ctx context.Context, f *cliFixture, id string) domain.Attempt {
	t.Helper()
	var got domain.Attempt
	decodeCLIData(t, concurrentCLIEnvelope(t, concurrentCLICommand(ctx, f, 0, "attempts", "get", id)), &got)
	return got
}
func routingWaitAttempt(t *testing.T, ctx context.Context, f *cliFixture, id, phase string) domain.Attempt {
	t.Helper()
	for ctx.Err() == nil {
		got := routingReadAttempt(t, ctx, f, id)
		if got.Phase == phase {
			return got
		}
		if got.Phase == "settled" {
			t.Fatalf("attempt settled before %s: %+v", phase, got)
		}
		select {
		case <-time.After(25 * time.Millisecond):
		case <-ctx.Done():
		}
	}
	t.Fatalf("routing fixture exceeded bounded wait for %s", phase)
	return domain.Attempt{}
}
func routingAttemptEvents(t *testing.T, ctx context.Context, f *cliFixture, id string) []domain.AttemptEvent {
	t.Helper()
	all := []domain.AttemptEvent{}
	cursor := ""
	for range 10 {
		args := []string{"attempts", "events", id, "--limit", "100"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		var page domain.Page[domain.AttemptEvent]
		decodeCLIData(t, concurrentCLIEnvelope(t, concurrentCLICommand(ctx, f, 0, args...)), &page)
		all = append(all, page.Items...)
		if page.NextCursor == nil {
			return all
		}
		cursor = *page.NextCursor
	}
	t.Fatal("routing fixture exceeded bounded history pages")
	return nil
}
