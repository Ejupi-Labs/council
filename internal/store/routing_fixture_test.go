package store

import (
	"context"
	"database/sql"
	_ "embed"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
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
//
// cleanSyntheticBrowserEpoch is limited to the two full=false browser seeds
// created in this test package. The seed process itself constructed the entire
// database under its still-held native lock and never launched an owner or child.
// It runs the actual old-schema migration, proves that migration retained the
// untracked epoch and all original rows, then supplies a synthetic clean marker
// solely for browser journeys that need admission. This is fixture provenance,
// never a production migration or inference from old on-disk state.
func cleanSyntheticBrowserEpoch(t *testing.T, path string, old *sql.DB, ids routingSchema3IDs, expectedVersion int) {
	t.Helper()
	var version int
	if err := old.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != expectedVersion {
		t.Fatal("the disposable browser fixture did not start from its exact old schema", version, err)
	}
	before := unaffectedRows(t, old)
	var oldRoutingPolicy string
	if expectedVersion == 4 {
		if err := old.QueryRow("SELECT data FROM routing_policy WHERE singleton=1").Scan(&oldRoutingPolicy); err != nil {
			t.Fatal("the accepted schema4 policy is unavailable", err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal("actual legacy migration failed", err)
	}
	defer s.Close()
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatal("the real legacy migration did not reach current schema", version, err)
	}
	if after := unaffectedRows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("the real migration changed historical fixture records")
	}
	if expectedVersion == 4 {
		var currentPolicy string
		if err = s.db.QueryRow("SELECT data FROM routing_policy WHERE singleton=1").Scan(&currentPolicy); err != nil || currentPolicy != oldRoutingPolicy {
			t.Fatal("the accepted schema4 routing policy changed", err)
		}
	}
	ctx := context.Background()
	epoch, err := readRecoveryOwnerEpoch(ctx, s.db)
	if err != nil || epoch.State != "untracked" || epoch.OwnerID != "" || epoch.Generation != 0 {
		t.Fatal("migration must not infer a clean prior owner", epoch, err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "open" || gate.OwnerInstanceID != "" || gate.IncidentID != nil || len(gate.ActiveClaims) != 0 {
		t.Fatal("the constructed fixture unexpectedly contains an owner, incident or claim", gate, err)
	}
	var attempts, unsettled, incidents, claims, operations int
	for _, check := range []struct {
		query string
		out   *int
	}{
		{"SELECT count(*) FROM attempts", &attempts},
		{"SELECT count(*) FROM attempts WHERE phase!='settled'", &unsettled},
		{"SELECT count(*) FROM execution_incidents", &incidents},
		{"SELECT count(*) FROM capacity_claims", &claims},
		{"SELECT count(*) FROM delivery_operations", &operations},
	} {
		if err = s.db.QueryRow(check.query).Scan(check.out); err != nil {
			t.Fatal("cannot verify the constructed fixture", err)
		}
	}
	if attempts != 1 || unsettled != 0 || incidents != 0 || claims != 0 || operations != 0 {
		t.Fatal("the synthetic fixture is not the known never-running case", attempts, unsettled, incidents, claims, operations)
	}
	attempt, err := s.attempt(ctx, s.db, ids.AttemptID)
	if err != nil || attempt.Phase != "settled" || attempt.LaunchIntent || attempt.Cleanup.State != "verified" || attempt.Cleanup.Scope != "not_launched" {
		t.Fatal("the exact fixture attempt is not verified never-launched", attempt.ID, err)
	}
	result, err := s.db.Exec("UPDATE recovery_owner_epoch SET state='clean' WHERE singleton=1 AND state='untracked' AND owner_instance_id='' AND generation=0")
	if err != nil {
		t.Fatal("cannot set test-only synthetic provenance", err)
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		t.Fatal("the synthetic marker changed an unexpected number of rows", n, e)
	}
	if after := unaffectedRows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("the test-only marker changed retained historical records")
	}
}

// This separate actual legacy migration deliberately has no synthetic clean
// marker. It proves the normal production behavior remains fail-closed.
func TestRecoveryBrowserOldSchemaWithoutSyntheticProvenanceFences(t *testing.T) {
	for _, schema := range []int{3, 4} {
		t.Run(domain.Decimal(int64(schema)), func(t *testing.T) {
			lock, err := localstate.Acquire(filepath.Join(t.TempDir(), "private"))
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			var old *sql.DB
			if schema == 3 {
				old, _ = routingSchema3Database(t, lock.DatabasePath(), "", false)
			} else {
				old, _ = workflowSchema4Database(t, lock.DatabasePath(), "", false)
			}
			before := unaffectedRows(t, old)
			if err := old.Close(); err != nil {
				t.Fatal(err)
			}
			s, err := Open(lock.DatabasePath())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if after := unaffectedRows(t, s.db); !reflect.DeepEqual(before, after) {
				t.Fatal("migration changed historical browser fixture rows")
			}
			ctx := context.Background()
			epoch, err := readRecoveryOwnerEpoch(ctx, s.db)
			if err != nil || epoch.State != "untracked" {
				t.Fatal("old state must remain untracked", epoch, err)
			}
			owner := domain.NewID()
			if err = s.BeginOwnerEpoch(ctx, owner); err != nil {
				t.Fatal(err)
			}
			if err = s.RecoverOwner(ctx, owner); err != nil {
				t.Fatal(err)
			}
			gate, err := s.Gate(ctx)
			if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
				t.Fatal("untracked legacy owner was not fenced", gate, err)
			}
			incident, err := s.Incident(ctx, *gate.IncidentID)
			if err != nil || incident.Code != "owner_epoch_uncertain" {
				t.Fatal("the untracked-owner incident is not observable", incident, err)
			}
		})
	}
}

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
	cleanSyntheticBrowserEpoch(t, path, db, ids, 3)
	raw, e := json.Marshal(ids)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(output, raw, 0600); e != nil {
		t.Fatal(e)
	}
}
