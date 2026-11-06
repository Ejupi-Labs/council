package domain

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"strconv"
)

// WorkflowAuthoritySHA256 hashes the captured typed authority, including its
// occurrence identity, with no map or request-default normalization.
func WorkflowAuthoritySHA256(authority WorkflowAuthority) (string, *Error) {
	body, err := json.Marshal(authority)
	if err != nil || len(body) > MaxWorkflowBytes {
		return "", Invalid("bounded canonical workflow authority")
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// WorkflowStageKey is shared by controller and DAO; it names a logical slot,
// never a particular execution or its 004A replacement.
func WorkflowStageKey(role string, cycle int, definitionID, subjectID string) (string, *Error) {
	if cycle < 0 || cycle > 1 {
		return "", Invalid("workflow cycle zero or one")
	}
	key := role + ":" + strconv.Itoa(cycle)
	switch role {
	case "author":
		if cycle != 0 || definitionID != "" || subjectID != "" {
			return "", Invalid("initial author logical slot")
		}
		return key, nil
	case "check":
		if !ValidID(definitionID) || !ValidID(subjectID) {
			return "", Invalid("check definition and selected subject IDs")
		}
		return key + ":" + definitionID + ":" + subjectID, nil
	case "correction":
		if cycle != 1 {
			return "", Invalid("correction cycle one")
		}
	case "review", "local_delivery", "external_delivery", "accept":
	default:
		return "", Invalid("supported workflow role")
	}
	if definitionID != "" || !ValidID(subjectID) {
		return "", Invalid("selected subject logical slot")
	}
	return key + ":" + subjectID, nil
}
