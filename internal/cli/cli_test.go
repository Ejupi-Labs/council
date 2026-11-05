package cli

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParserRejectsAmbiguousCommands(t *testing.T) {
	for _, args := range [][]string{
		{"drafts", "create", "--project", "x", "--task", "a", "--task", "b"},
		{"policy", "set", "autonomous"},
		{"projects", "list", "--unknown"},
		{"projects", "list", "--api-version", "2"},
		{"drafts", "create", "--project", "x", "--task", "a", "--task-file", "b"},
	} {
		if _, err := parse(args, nil); err == nil {
			t.Errorf("accepted ambiguous input %v", args)
		}
	}
}

func TestOversizedTaskFilePrecedesTruncatedUTF8Validation(t *testing.T) {
	_, err := parse([]string{"drafts", "create", "--project", strings.Repeat("1", 32), "--task-file", "-"}, strings.NewReader(strings.Repeat("x", domain.MaxTextBytes)+"é"))
	if err == nil || err.Code != "input_too_large" {
		t.Fatalf("wrong size error: %v", err)
	}
}

func TestForwardRequiresAnAuthoritativeMutationReceipt(t *testing.T) {
	id := strings.Repeat("1", 32)
	for _, kind := range []string{"missing", "wrong_operation", "wrong_id", "invalid_time", "error_receipt", "valid"} {
		t.Run(kind, func(t *testing.T) {
			response := domain.Success(201, id, map[string]string{"id": strings.Repeat("2", 32)}, &domain.Receipt{RequestID: id, Operation: "drafts.create", CommittedAt: domain.Now()})
			switch kind {
			case "missing":
				response.Envelope.Receipt = nil
			case "wrong_operation":
				response.Envelope.Receipt.Operation = "policy.set"
			case "wrong_id":
				response.Envelope.Receipt.RequestID = strings.Repeat("3", 32)
			case "invalid_time":
				response.Envelope.Receipt.CommittedAt = "yesterday"
			case "error_receipt":
				response = domain.Failure(id, domain.Invalid("draft input"))
				response.Envelope.Receipt = &domain.Receipt{RequestID: id}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Origin") == "" || r.Header.Get("Authorization") != "Bearer token" {
					t.Error("forwarded authority missing")
				}
				body, _ := json.Marshal(response.Envelope)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(response.Status)
				_, _ = w.Write(body)
			}))
			defer server.Close()
			result := forward(context.Background(), localstate.Descriptor{Origin: server.URL, Token: "token"}, command{method: "POST", op: "drafts.create", route: "/drafts", requestID: id})
			if kind == "valid" {
				if result.Envelope.Error != nil {
					t.Fatal(result.Envelope.Error)
				}
			} else if result.Envelope.Error == nil || result.Envelope.Error.Code != "write_uncertain" || result.Envelope.RequestID != id {
				t.Fatalf("malformed acknowledgement treated as authoritative: %+v", result)
			}
		})
	}
}

func TestInvalidJSONCLIAlwaysReturnsOneEnvelope(t *testing.T) {
	for _, args := range [][]string{{"--json", "projects", "list", "--bogus"}, {"projects", "list", "--bogus", "--json"}, {"drafts", "list", "--project", "", "--json"}, {"projects", "open", ".", "--request-id", "", "--json"}, {"--version", "--no-open", "--json"}} {
		var out, diagnostics bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(""), &out, &diagnostics)
		if code != 2 || !strings.Contains(out.String(), `"error"`) || strings.Count(out.String(), "\n") != 1 || diagnostics.Len() != 0 {
			t.Errorf("bad JSON failure for %v: code%d stdout=%q stderr=%q", args, code, out.String(), diagnostics.String())
		}
	}
}
