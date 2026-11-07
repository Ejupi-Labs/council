package domain

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
)

// GraphEdgeID is the stable semantic identity of one declared versioned edge.
func GraphEdgeID(graphID, version, fromNodeID, toNodeID, kind string) (string, error) {
	if !ValidID(graphID) || !ValidID(fromNodeID) || !ValidID(toNodeID) {
		return "", Invalid("canonical graph and endpoint identities")
	}
	if _, invalid := ParseRevision(version); invalid != nil {
		return "", invalid
	}
	if kind != "files" && kind != "context" {
		return "", Invalid("typed graph dependency kind")
	}
	body, err := json.Marshal(struct {
		GraphID      string `json:"graphId"`
		GraphVersion string `json:"graphVersion"`
		FromNodeID   string `json:"fromNodeId"`
		ToNodeID     string `json:"toNodeId"`
		Kind         string `json:"kind"`
	}{graphID, version, fromNodeID, toNodeID, kind})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:16]), nil
}
