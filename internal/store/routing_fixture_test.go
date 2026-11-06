package store

import (
	"context"
	"database/sql"
	_ "embed"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

//go:embed testdata/schema3.sql
var acceptedRoutingSchema3 string

type routingSchema3IDs struct {
	ProjectID        string                `json:"projectId"`
	TaskID           string                `json:"taskId"`
	AttemptID        string                `json:"attemptId"`
	ActiveAttemptID  string                `json:"activeAttemptId"`
	RequestID        string                `json:"requestId"`
	OriginalText     string                `json:"originalText"`
	ExpectedRevision string                `json:"expectedRevision"`
	Input            domain.StartTaskInput `json:"-"`
}

// routingSchema3Database uses frozen accepted DDL and direct old records, never
// the current Open/CreateTask defaults. All metadata is local fixture data.
func routingSchema3Database(t *testing.T, path, selectedPath string, full bool) (*sql.DB, routingSchema3IDs) {
	return routingLegacyDatabase(t, path, selectedPath, full, acceptedRoutingSchema3)
}
func routingLegacyDatabase(t *testing.T, path, selectedPath string, full bool, ddl string) (*sql.DB, routingSchema3IDs) {
	t.Helper()
	dsn, e := sqliteDSN(path)
	if e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, e = db.Exec(ddl); e != nil {
		db.Close()
		t.Fatal(e)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(q, args...); e != nil {
			t.Fatal(q, e)
		}
	}
	now := "2026-11-06T00:00:00.000Z"
	large := "9007199254740993"
	ids := routingSchema3IDs{ProjectID: strings.Repeat("1", 32), TaskID: strings.Repeat("2", 32), AttemptID: strings.Repeat("3", 32), RequestID: strings.Repeat("4", 32), OriginalText: "\ufeff Preserve original café \ufffd\r\n東京 instructions exactly.\n", ExpectedRevision: "1"}
	policy := domain.Policy{Mode: "autonomous", SelectionSource: "user", Revision: "2", UpdatedAt: now}
	exec("INSERT INTO policy VALUES(1,2,?)", encode(policy))
	p := domain.Project{ID: ids.ProjectID, SelectedPath: selectedPath, ResolvedPath: selectedPath, RepositoryRoot: selectedPath, CreatedAt: now, UpdatedAt: now, ObservedAt: now, Revision: "1"}
	identity := "schema3-local-fixture"
	if selectedPath != "" {
		inspection, e := project.Inspect(context.Background(), selectedPath, selectedPath)
		if e != nil {
			t.Fatal(e)
		}
		p.ResolvedPath = inspection.Observation.ResolvedPath
		p.RepositoryRoot = inspection.Observation.RepositoryRoot
		p.Branch = inspection.Observation.Branch
		p.Commit = inspection.Observation.Commit
		identity = inspection.Identity
	}
	exec("INSERT INTO projects VALUES(1,?,?,?,?)", p.ID, identity, p.ResolvedPath, encode(p))
	draftID := strings.Repeat("5", 32)
	draft := domain.Draft{ID: draftID, ProjectID: p.ID, OriginalText: ids.OriginalText, Revision: "1", Status: "open", Selection: "auto", RequestedPolicy: policy, CreatedAt: now, UpdatedAt: now}
	exec("INSERT INTO drafts VALUES(1,?,?,1)", draftID, p.ID)
	exec("INSERT INTO draft_versions VALUES(?,1,?)", draftID, encode(draft))
	input := domain.StartTaskInput{RequestID: ids.RequestID, ProjectID: p.ID, OriginalText: ids.OriginalText, SourceDraftRef: &domain.DraftRef{DraftID: draftID, Revision: "1"}, ExecutionMode: "fake", FakeScenario: "response"}
	ids.Input = input
	version := domain.TaskVersion{TaskID: ids.TaskID, Version: "1", CreatedAt: now, CreatedBy: "user", RequestedPolicy: policy, TimeoutSeconds: 1800, Acceptance: domain.TaskAcceptance{Deliverable: "response_only", RequiredEvidence: []string{}, AllowCandidatePreparation: true}}
	unknown := domain.ObservedFact{Status: "unknown"}
	a := domain.Attempt{ID: ids.AttemptID, TaskID: ids.TaskID, TaskVersion: "1", StartRequestID: ids.RequestID, OwnerInstanceID: strings.Repeat("6", 32), ConnectionID: "codex-native", ExecutionMode: "fake", FakeScenario: "response", Phase: "settled", Outcome: ptr("blocked"), Revision: "5", AdmittedAt: now, DeadlineAt: "2026-11-06T00:30:00.000Z", SettledAt: &now, ObservedAt: now, RequestedTarget: version.Pins, TargetObservations: domain.TargetObservations{Provider: unknown, Account: unknown, Model: unknown, NativeSession: unknown, Permissions: domain.PermissionObservation{RequestedMode: "autonomous", Limitations: []string{"Accepted003 local fixture; no provider was invoked."}}, Usage: domain.UsageObservation{Completeness: "unknown"}}, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}
	task := domain.Task{ID: ids.TaskID, ProjectID: p.ID, OriginalText: ids.OriginalText, SourceDraftRef: input.SourceDraftRef, CurrentVersion: "1", Revision: "1", StateRevision: "5", LatestAttemptID: &a.ID, CreatedAt: now, UpdatedAt: now}
	if full {
		task.Revision = large
		task.StateRevision = "9007199254740994"
		ids.ExpectedRevision = large
	}
	exec("INSERT INTO tasks VALUES(1,?,?,?,?,1,?)", task.ID, p.ID, mustFixtureCount(t, task.Revision), mustFixtureCount(t, task.StateRevision), encode(task))
	exec("INSERT INTO task_versions VALUES(?,1,?)", task.ID, encode(version))
	exec("INSERT INTO attempts VALUES(1,?,?,1,5,'settled',?)", a.ID, task.ID, encode(a))
	exec("INSERT INTO attempt_terminals VALUES(?,?)", a.ID, encode(map[string]any{"attempt": a, "fixture": "accepted003"}))
	event := domain.AttemptEvent{Sequence: "9007199254740993", AttemptID: a.ID, Kind: "settled", ObservedAt: now, Metadata: []byte(`{"fixture":"schema3","text":"\ufeffcafé\r\n"}`)}
	exec("INSERT INTO attempt_events VALUES(?,?,?)", int64(9007199254740993), a.ID, encode(event))
	admitted := a
	admitted.Phase = "queued"
	admitted.Outcome = nil
	admitted.SettledAt = nil
	admitted.Revision = "1"
	receipt := &domain.Receipt{RequestID: ids.RequestID, Operation: "tasks.start", CommittedAt: now}
	envelope := domain.Success(201, ids.RequestID, domain.Admission{Task: taskSummary(task, version, &admitted, false), Version: version, Attempt: admitted}, receipt).Envelope
	exec("INSERT INTO receipts VALUES(?,?,?,?,?)", ids.RequestID, "tasks.start", TaskStartFingerprint(input), 201, encode(envelope))
	if full {
		// Populate immutable source/candidate/artifact references without creating
		// actual work. These intentionally labelled metadata records are never served
		// as a verified file by browser fixtures (which use full=false).
		hash := strings.Repeat("a", 64)
		artifactIDs := []string{strings.Repeat("7", 32), strings.Repeat("8", 32), strings.Repeat("9", 32), strings.Repeat("a", 32), strings.Repeat("b", 32)}
		for i, kind := range []string{"source_manifest", "index_snapshot", "candidate_manifest", "candidate_delta", "final_response"} {
			m := domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: artifactIDs[i], AttemptID: a.ID, Kind: kind, ByteLength: large, Closed: true, Completeness: "complete", SHA256: &hash, CreatedAt: now}, PrivateRef: "artifacts/" + artifactIDs[i] + ".bin", PhysicalIdentity: "labelled-schema3-fixture"}
			exec("INSERT INTO artifacts VALUES(?,?,?,?,?,?,?)", i+1, m.ID, a.ID, m.PrivateRef, m.PhysicalIdentity, int64(9007199254740993), encode(m.ArtifactMetadata))
		}
		snapshot := domain.SourceSnapshot{ID: strings.Repeat("c", 32), AttemptID: a.ID, ManifestArtifactID: artifactIDs[0], IndexArtifactID: artifactIDs[1], ManifestSHA256: hash, CreatedAt: now}
		exec("INSERT INTO source_snapshots VALUES(?,?,?,?,?)", snapshot.ID, a.ID, snapshot.ManifestArtifactID, snapshot.IndexArtifactID, encode(snapshot))
		candidate := domain.Candidate{ID: strings.Repeat("d", 32), AttemptID: a.ID, SourceSnapshotID: snapshot.ID, ManifestArtifactID: artifactIDs[2], DeltaArtifactID: artifactIDs[3], WorkspaceRef: "workspaces/accepted003-fixture", Verification: "pending", Integration: "not_performed", CreatedAt: now}
		exec("INSERT INTO candidates VALUES(?,?,?,?,?,?,?)", candidate.ID, a.ID, snapshot.ID, candidate.ManifestArtifactID, candidate.DeltaArtifactID, candidate.WorkspaceRef, encode(candidate))
		accept := domain.ResponseAcceptance{ID: strings.Repeat("e", 32), TaskID: task.ID, TaskVersion: "1", AttemptID: a.ID, ResponseArtifactID: artifactIDs[4], ResponseSHA256: hash, Basis: "user_review", Confirmation: "reviewed_response_and_conditions", AcceptedAt: now}
		exec("INSERT INTO response_acceptances VALUES(?,?,?,?,?,?)", accept.ID, task.ID, 1, a.ID, accept.ResponseArtifactID, encode(accept))
		activeTask := task
		activeTask.ID = strings.Repeat("f", 32)
		active := a
		active.ID = "abcdefabcdefabcdefabcdefabcdefab"
		active.TaskID = activeTask.ID
		active.Phase = "preparing"
		active.Outcome = nil
		active.SettledAt = nil
		active.Revision = "1"
		active.LaunchIntent = true
		active.Cleanup = domain.Cleanup{State: "unknown", Scope: "windows_job"}
		activeTask.LatestAttemptID = &active.ID
		ids.ActiveAttemptID = active.ID
		exec("INSERT INTO tasks VALUES(2,?,?,?,?,1,?)", activeTask.ID, p.ID, mustFixtureCount(t, activeTask.Revision), mustFixtureCount(t, activeTask.StateRevision), encode(activeTask))
		v := version
		v.TaskID = activeTask.ID
		exec("INSERT INTO task_versions VALUES(?,1,?)", activeTask.ID, encode(v))
		exec("INSERT INTO attempts VALUES(2,?,?,1,1,'preparing',?)", active.ID, activeTask.ID, encode(active))
		incident := domain.ExecutionIncident{ID: "fedcbafedcbafedcbafedcbafedcbafedc", AttemptID: &active.ID, Code: "fixture_retained_unknown", Message: "Accepted003 fixture retains uncertain owned cleanup; no real process exists.", CreatedAt: now}
		exec("INSERT INTO execution_incidents VALUES(?,?,?)", incident.ID, active.ID, encode(incident))
		exec("UPDATE execution_control SET state='fenced',owner_instance_id=?,incident_id=?", a.OwnerInstanceID, incident.ID)
		exec("INSERT INTO capacity_claims VALUES('managed-native',?)", active.ID)
	}
	return db, ids
}
func mustFixtureCount(t *testing.T, s string) int64 {
	t.Helper()
	n, e := domain.ParseCount(s)
	if e != nil {
		t.Fatal(e)
	}
	return n
}

// Test-only actual-schema seed for a disposable browser/CLI project. It never
// opens user state and refuses an existing nonempty database. The caller supplies
// an isolated root/project/output path; stdout remains ordinary go-test output.
func TestRoutingSchema3SeedChild(t *testing.T) {
	path := os.Getenv("COUNCIL_ROUTING_SCHEMA3_SEED_PATH")
	if path == "" {
		t.Skip("explicit disposable schema3 seed helper")
	}
	selected := os.Getenv("COUNCIL_ROUTING_SCHEMA3_PROJECT")
	output := os.Getenv("COUNCIL_ROUTING_SCHEMA3_SEED_OUTPUT")
	if !filepath.IsAbs(path) || filepath.Base(path) != "workspace.sqlite3" || !filepath.IsAbs(selected) || !filepath.IsAbs(output) {
		t.Fatal("absolute disposable database/project/output paths required")
	}
	if info, e := os.Stat(path); e == nil && info.Size() != 0 {
		t.Fatal("refusing an existing nonempty database")
	}
	lock, e := localstate.Acquire(filepath.Dir(path))
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	db, ids := routingSchema3Database(t, path, selected, false)
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(ids)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(output, raw, 0600); e != nil {
		t.Fatal(e)
	}
}
