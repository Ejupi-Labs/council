package store

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphVersionOptionsFixture(t *testing.T) (*Store, domain.GraphAdmission, domain.GraphNodeExecutionOptions, graphTaskVersionRecord) {
	t.Helper()
	checks := []domain.CheckDeclaration{}
	zero := 0
	options := &domain.WorkflowInput{Review: "auto", Checks: &checks, MaxCorrectionCycles: &zero}
	s, graph, _, _, _ := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.Nodes[0].Workflow = options
		in.Nodes[0].FakeScenario = "graph-response"
	})
	n := graph.Nodes[0]
	first := domain.GraphNodeExecutionOptions{GraphID: graph.Graph.ID, NodeID: n.ID, TaskID: n.TaskID, TaskVersion: n.TaskVersion, ExecutionMode: "fake", FakeScenario: "graph-response", Workflow: options}
	task, e := s.Task(context.Background(), n.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	v := task.Version
	v.Version, v.CreatedBy, v.GuidanceText = "2", "user_guidance", "Exact second-version guidance"
	v.CreatedAt = domain.Now()
	second := first
	second.TaskVersion = "2"
	changed := *options
	changed.Review = "required"
	second.Workflow = &changed
	return s, graph, first, graphTaskVersionRecord{TaskVersion: v, GraphOptions: &second}
}

// This explicitly labelled immutable-version metadata fixture qualifies the
// private codec/read boundary, not the not-yet-enabled guidance transition.
func installGraphOptionsVersion(t *testing.T, s *Store, graph domain.GraphAdmission, record graphTaskVersionRecord) {
	t.Helper()
	e := s.executionWrite(context.Background(), func(tx *sql.Tx) error {
		if _, e := tx.Exec("INSERT INTO task_versions VALUES(?,?,?)", record.TaskID, intRevision(record.Version), encode(record)); e != nil {
			return e
		}
		task, e := s.task(context.Background(), tx, record.TaskID)
		if e != nil {
			return e
		}
		task.CurrentVersion, task.Revision = record.Version, record.Version
		if e = saveTask(context.Background(), tx, task); e != nil {
			return e
		}
		nodes, e := s.graphNodes(context.Background(), tx, graph.Graph.ID)
		if e != nil {
			return e
		}
		for _, n := range nodes {
			if n.Summary.TaskID == task.ID {
				n.Summary.TaskVersion, n.Summary.TaskRevision = record.Version, record.Version
				return saveGraphNode(context.Background(), tx, n)
			}
		}
		return notFound()
	})
	if e != nil {
		t.Fatal(e)
	}
}

func TestGraphNodeOptionsPreservesInitialAndHistoricalExactChoicesPrivately(t *testing.T) {
	s, graph, expected, second := graphVersionOptionsFixture(t)
	ctx := context.Background()
	var original string
	if e := s.db.QueryRow("SELECT data FROM task_versions WHERE task_id=? AND version=1", expected.TaskID).Scan(&original); e != nil {
		t.Fatal(e)
	}
	first, e := s.GraphNodeOptions(ctx, expected.GraphID, expected.NodeID, "1")
	if e != nil || !reflect.DeepEqual(first, expected) || first.Workflow == nil || first.Workflow.Checks == nil || len(*first.Workflow.Checks) != 0 {
		t.Fatal("initial exact omitted/empty choices were unavailable or normalised", first, e)
	}
	installGraphOptionsVersion(t, s, graph, second)
	current, e := s.GraphNodeOptions(ctx, expected.GraphID, expected.NodeID, "2")
	if e != nil || !reflect.DeepEqual(current, *second.GraphOptions) {
		t.Fatal("second version reused initial workflow options", current, second.GraphOptions, e)
	}
	historical, e := s.GraphNodeOptions(ctx, expected.GraphID, expected.NodeID, "1")
	if e != nil || !reflect.DeepEqual(historical, expected) {
		t.Fatal("historical options followed current version", historical, e)
	}
	public, e := s.Task(ctx, expected.TaskID)
	if e != nil || public.Version.Version != "2" || strings.Contains(encode(public.Version), "graphOptions") || strings.Contains(encode(public.Version), "fakeScenario") {
		t.Fatal("private graph envelope leaked into public TaskVersion", public, e)
	}
	var retained string
	if e = s.db.QueryRow("SELECT data FROM task_versions WHERE task_id=? AND version=1", expected.TaskID).Scan(&retained); e != nil || retained != original {
		t.Fatal("options reads rewrote historical version bytes", e)
	}
}

func TestGraphNodeOptionsRejectsForeignMalformedOrUnboundedPrivateVersion(t *testing.T) {
	for _, kind := range []string{"foreign_graph", "foreign_node", "missing_version", "wrong_tuple", "missing_options", "unknown_member", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			s, graph, expected, second := graphVersionOptionsFixture(t)
			installGraphOptionsVersion(t, s, graph, second)
			graphID, nodeID, version := graph.Graph.ID, expected.NodeID, "2"
			raw := encode(second)
			switch kind {
			case "foreign_graph":
				graphID = domain.NewID()
			case "foreign_node":
				nodeID = graph.Nodes[1].ID
			case "missing_version":
				version = "3"
			case "wrong_tuple":
				second.GraphOptions.NodeID = graph.Nodes[1].ID
				raw = encode(second)
			case "missing_options":
				second.GraphOptions = nil
				raw = encode(second)
			case "unknown_member":
				raw = strings.TrimSuffix(raw, "}") + `,"unrecognisedAuthority":true}`
			case "oversized":
				// One allocation: coherent JSON plus whitespace crosses the full
				// serialized-version bound by exactly one byte.
				var oversized strings.Builder
				oversized.Grow(maxGraphVersionRecordBytes + 1)
				oversized.WriteString(raw)
				const spaces = "                                                                "
				for oversized.Len()+len(spaces) <= maxGraphVersionRecordBytes+1 {
					oversized.WriteString(spaces)
				}
				for oversized.Len() <= maxGraphVersionRecordBytes {
					oversized.WriteByte(' ')
				}
				raw = oversized.String()
			}
			if _, e := s.db.Exec("UPDATE task_versions SET data=? WHERE task_id=? AND version=2", raw, expected.TaskID); e != nil {
				t.Fatal(e)
			}
			if options, e := s.GraphNodeOptions(context.Background(), graphID, nodeID, version); e == nil {
				t.Fatal("foreign or malformed version options were accepted", kind, options)
			}
		})
	}
}

func TestGraphNodeOptionsRetainsValidRoutingGuidanceSnapshot(t *testing.T) {
	s, graph, expected, second := graphVersionOptionsFixture(t)
	useCurrent := true
	second.GraphOptions.Routing = &domain.RoutingOptions{UseCurrentPolicy: &useCurrent, ExpectedPolicyRevision: "1", Suitability: "routine"}
	if invalid := domain.ValidateRoutingOptions(second.GraphOptions.Routing, "guidance"); invalid != nil {
		t.Fatal("fixture routing guidance is invalid", invalid)
	}
	installGraphOptionsVersion(t, s, graph, second)
	actual, e := s.GraphNodeOptions(context.Background(), expected.GraphID, expected.NodeID, "2")
	if e != nil || !reflect.DeepEqual(actual, *second.GraphOptions) {
		t.Fatal("valid versioned routing guidance was interpreted as execution mode", actual, e)
	}
}
