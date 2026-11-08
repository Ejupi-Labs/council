package domain

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
)

// EncodeHistoryExport serializes only the captured, validated metadata projection.
// The digest binds the exact canonical inner document; no source payload or path
// is consulted. It deliberately does not grant authority to import or resume it.
func EncodeHistoryExport(snapshot HistorySnapshot) ([]byte, *Error) {
	if invalid := ValidateHistorySnapshot(snapshot); invalid != nil {
		return nil, invalid
	}
	canonical := func(value any) ([]byte, error) {
		// Convert structs into JSON object maps so deterministic marshaling also
		// orders their keys lexicographically, including nested record fields.
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var object any
		if err = json.Unmarshal(encoded, &object); err != nil {
			return nil, err
		}
		return json.Marshal(object, json.Deterministic(true))
	}
	inner, err := canonical(snapshot)
	if err != nil {
		return nil, Fail("internal_error", "The history document could not be encoded.")
	}
	sum := sha256.Sum256(inner)
	payload := HistoryPayload{
		Format:    HistoryOutputFormat,
		Snapshot:  snapshot.Summary,
		Tasks:     snapshot.Tasks,
		Attempts:  snapshot.Attempts,
		Integrity: HistoryIntegrity{Algorithm: "sha256", Encoding: "hex-lower", SnapshotSHA256: hex.EncodeToString(sum[:])},
	}
	encoded, err := canonical(payload)
	if err != nil {
		return nil, Fail("internal_error", "The history document could not be encoded.")
	}
	if len(encoded) > HistoryMaxPayloadBytes {
		return nil, Fail("history_too_large", "The history document exceeds the supported output limit.")
	}
	return encoded, nil
}
