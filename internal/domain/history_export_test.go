package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestHistoryExportCanonicalAndVerifiable(t *testing.T) {
	snapshot := historyTestSnapshot()
	output, invalid := EncodeHistoryExport(snapshot)
	if invalid != nil {
		t.Fatal(invalid)
	}
	again, invalid := EncodeHistoryExport(snapshot)
	if invalid != nil || !bytes.Equal(output, again) {
		t.Fatal("export is not deterministic", invalid)
	}
	if bytes.ContainsAny(output, "\n\r\t") || !bytes.HasPrefix(output, []byte(`{"attempts":`)) {
		t.Fatalf("output is not compact lexicographically ordered JSON: %s", output)
	}
	var document map[string]any
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	integrity, ok := document["integrity"].(map[string]any)
	if !ok || integrity["algorithm"] != "sha256" || integrity["encoding"] != "hex-lower" {
		t.Fatal("invalid integrity descriptor", integrity)
	}
	inner, err := json.Marshal(map[string]any{"snapshot": document["snapshot"], "tasks": document["tasks"], "attempts": document["attempts"]}, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(inner)
	if integrity["snapshotSha256"] != hex.EncodeToString(digest[:]) {
		t.Fatal("digest does not bind the exported metadata")
	}
	for _, forbidden := range []string{"destinationPath", "requestId", "ownerToken", "artifactPayload"} {
		if strings.Contains(string(output), `"`+forbidden+`"`) {
			t.Fatalf("unexpected private field %s", forbidden)
		}
	}
}

func TestHistoryExportRejectsInvalidSnapshot(t *testing.T) {
	snapshot := historyTestSnapshot()
	snapshot.Attempts[0].TaskID = historyAttemptID
	if _, invalid := EncodeHistoryExport(snapshot); invalid == nil {
		t.Fatal("foreign attempt was exported")
	}
}

func TestHistoryExportAttemptLimitAndBoundedWorstCase(t *testing.T) {
	snapshot := historyTestSnapshot()
	snapshot.Attempts = make([]HistoryAttemptRow, HistoryMaxAttempts)
	const maxRevision = "9223372036854775807"
	for i := range snapshot.Attempts {
		snapshot.Attempts[i] = HistoryAttemptRow{
			Ordinal: strconv.Itoa(i + 1), AttemptID: fmt.Sprintf("%032x", i+1),
			TaskID: snapshot.Tasks[0].TaskID, TaskVersion: maxRevision,
			Revision: maxRevision, Phase: "cancelling",
		}
	}
	snapshot.Summary.AttemptCount = strconv.Itoa(len(snapshot.Attempts))
	output, invalid := EncodeHistoryExport(snapshot)
	if invalid != nil || len(output) > HistoryMaxPayloadBytes {
		t.Fatalf("maximum supported attempt inventory did not fit the output bound: %d %v", len(output), invalid)
	}
	snapshot.Attempts = append(snapshot.Attempts, HistoryAttemptRow{Ordinal: strconv.Itoa(HistoryMaxAttempts + 1), AttemptID: fmt.Sprintf("%032x", HistoryMaxAttempts+1), TaskID: snapshot.Tasks[0].TaskID, TaskVersion: "1", Revision: "1", Phase: "settled"})
	snapshot.Summary.AttemptCount = strconv.Itoa(len(snapshot.Attempts))
	if _, invalid = EncodeHistoryExport(snapshot); invalid == nil {
		t.Fatal("attempt limit overflow was exported")
	}
}
