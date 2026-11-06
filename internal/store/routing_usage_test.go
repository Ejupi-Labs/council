package store

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are explicitly synthetic retained observation shapes, never an executed
// vendor or fixture fact. Native-mode rows exercise the same strict registration
// and observation checks, exact projection and worst-case encoded page bounds.
func routingUsageRecords(t *testing.T, s *Store, base domain.Admission, n int) []string {
	t.Helper()
	ctx := context.Background()
	ids := []string{}
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		for range n {
			a := base.Attempt
			a.ID = domain.NewID()
			a.ExecutionMode = "native"
			a.Phase = "settled"
			a.Outcome = ptr("failed")
			a.RoutingDecisionID = ptr(domain.NewID())
			a.Cleanup.State = "verified"
			a.StartRequestID = domain.NewID()
			stdoutID := domain.NewID()
			max, zero, cost, source, now := "9223372036854775807", "0", "1e999", strings.Repeat("\x01", 128), domain.Now()
			a.TargetObservations.Usage = domain.UsageObservation{Scope: "top_level", Source: &source, ObservedAt: &now, InputTokens: &max, OutputTokens: &zero, CachedInputTokens: &zero, CacheCreationInputTokens: &zero, Completeness: "complete"}
			models := &domain.NativeModelsObservation{Scope: "whole_tree", Source: source, ObservedAt: now, Completeness: "complete", EvidenceArtifactIDs: []string{stdoutID}, OmittedModelCount: &zero, Models: []domain.NativeModelUsage{}}
			for i := range 16 {
				models.Models = append(models.Models, domain.NativeModelUsage{Model: fmt.Sprintf("%02d", i) + strings.Repeat("\x01", 96), InputTokens: &max, OutputTokens: &zero, CachedInputTokens: &zero, CacheCreationInputTokens: &zero, CostEstimateUSD: &cost})
			}
			a.TargetObservations.NativeModels = models
			if len(encode(a.TargetObservations)) > domain.MaxEventBytes {
				t.Fatal("synthetic observation is outside accepted metadata bound", len(encode(a.TargetObservations)))
			}
			if e := s.insertAttempt(ctx, tx, a); e != nil {
				return e
			}
			artifact := domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: stdoutID, AttemptID: a.ID, Kind: "stdout", ByteLength: "0", Closed: true, Completeness: "complete", SHA256: ptr(strings.Repeat("0", 64)), CreatedAt: now}, PrivateRef: "artifacts/" + stdoutID + ".bin", PhysicalIdentity: "synthetic-observation-record"}
			sequence, e := nextSequence(ctx, tx, "artifacts")
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "INSERT INTO artifacts VALUES(?,?,?,?,?,?,?)", sequence, stdoutID, a.ID, artifact.PrivateRef, artifact.PhysicalIdentity, 0, encode(artifact.ArtifactMetadata)); e != nil {
				return e
			}
			if _, e = s.checkedObservations(ctx, tx, a, a.TargetObservations); e != nil {
				return e
			}
			d := *base.RoutingDecision
			d.ID = *a.RoutingDecisionID
			d.AttemptID = a.ID
			d.RequestID = a.StartRequestID
			copied, e := decode[domain.RoutingDecision](encode(d))
			if e != nil {
				return e
			}
			d = copied
			for i := range d.CandidateAssessments {
				candidate := &d.CandidateAssessments[i]
				if candidate.ProfileSnapshot != nil && candidate.ProfileID != nil && d.SelectedTarget.ProfileID != nil && *candidate.ProfileID == *d.SelectedTarget.ProfileID {
					candidate.ProfileSnapshot.ReferencePrice = &domain.ReferencePrice{Currency: "USD", Unit: ptr("per_million_tokens"), Basis: ptr("api_list"), InputPerMillion: ptr("1e999"), OutputPerMillion: ptr("1"), SourceKind: "user_reference", SourceLabel: strings.Repeat("\"", 128), SourceURL: ptr(strings.Repeat("\"", 1024)), ObservedAt: now}
				}
			}
			sequence, e = nextSequence(ctx, tx, "routing_decisions")
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "INSERT INTO routing_decisions VALUES(?,?,?,?,?,?,?,?)", sequence, d.ID, d.TaskID, intRevision(d.TaskVersion), d.AttemptID, d.PreviousAttemptID, d.Kind, encode(d)); e != nil {
				return e
			}
			ids = append(ids, a.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}
func TestRoutingUsageWholeItemBudgetStableCursorAndExactPageSubtotal(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "usage")
	r := s.CreateTask(ctx, domain.NewID(), taskInput(p.ID))
	if r.Status != 201 {
		t.Fatal(r)
	}
	base := r.Envelope.Data.(domain.Admission)
	routingUsageRecords(t, s, base, 100)
	first, e := s.Usage(ctx, base.Task.ID, 100, "")
	if e != nil {
		t.Fatal(e)
	}
	if len(first.Items) == 0 || len(first.Items) >= 100 || first.NextCursor == nil || len(encode(first)) > domain.MaxUsagePageBytes {
		t.Fatal("byte-budget paging did not end before next whole item", len(first.Items), len(encode(first)), e)
	}
	late := routingUsageRecords(t, s, base, 1)[0]
	seen := map[string]bool{}
	last := int64(0)
	native := 0
	page := first
	for {
		pageNative := 0
		for _, item := range page.Items {
			if seen[item.AttemptID] || item.AttemptID == late {
				t.Fatal("cursor duplicated/skipped into a newer high-water row", item.AttemptID)
			}
			seen[item.AttemptID] = true
			seq, _ := strconv.ParseInt(item.AttemptSequence, 10, 64)
			if seq <= last {
				t.Fatal("out-of-order usage", seq, last)
			}
			last = seq
			if item.ExecutionMode != "fake" {
				native++
				pageNative++
			}
		}
		if len(encode(page)) > domain.MaxUsagePageBytes || page.KnownSubtotal.Scope != "returned_attempts" || page.Coverage.Scope != "returned_attempts" || page.Charges.Status != "unknown" {
			t.Fatal("usage page claimed an unbounded total or charge", page.Coverage)
		}
		want := new(big.Int).Mul(big.NewInt(int64(pageNative)), big.NewInt(9223372036854775807)).String()
		if page.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue == nil || *page.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue != want {
			t.Fatal("subtotal overflowed or included omitted rows", want, page.KnownSubtotal)
		}
		for _, estimate := range page.Estimates {
			if estimate.Value != "1e999" {
				t.Fatal("native estimate clipped", estimate)
			}
		}
		if page.NextCursor == nil {
			break
		}
		page, e = s.Usage(ctx, base.Task.ID, 100, *page.NextCursor)
		if e != nil {
			t.Fatal(e)
		}
	}
	if native != 100 || len(seen) != 101 {
		t.Fatal("usage paging lost retained attempts", native, len(seen))
	}
	otherResult := s.CreateTask(ctx, domain.NewID(), taskInput(p.ID))
	if otherResult.Status != 201 {
		t.Fatal(otherResult)
	}
	other := otherResult.Envelope.Data.(domain.Admission)
	if _, e = s.Usage(ctx, other.Task.ID, 100, *first.NextCursor); e == nil {
		t.Fatal("foreign task cursor accepted")
	}
}
