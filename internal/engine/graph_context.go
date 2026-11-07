package engine

import (
	json "encoding/json/v2"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// graphScopedConditions constructs a receiver's exact instruction provenance.
// It does not claim physical artifact closure, membership or a passed verdict;
// those facts are independently checked before the store captures authority.
func graphScopedConditions(graphID, graphVersion, taskID, taskVersion string, sources []domain.GraphConditionSource) (domain.GraphConditionsBinding, error) {
	var zero domain.GraphConditionsBinding
	if !domain.ValidID(graphID) || !domain.ValidID(taskID) || len(sources) < 2 || len(sources) > domain.MaxGraphNodes+1 {
		return zero, domain.Invalid("complete receiver condition sources")
	}
	for _, version := range []string{graphVersion, taskVersion} {
		if _, invalid := domain.ParseRevision(version); invalid != nil {
			return zero, invalid
		}
	}
	seenRefs, seenNodes := map[string]bool{}, map[string]bool{}
	shared, receiver := 0, 0
	sealed := make([]domain.GraphConditionSource, 0, len(sources))
	for _, source := range sources {
		if source.Ref == "" || len(source.Ref) > 128 || !utf8.ValidString(source.Ref) || strings.ContainsRune(source.Ref, 0) || seenRefs[source.Ref] || !validGraphArtifactReference(source.OriginalRef) {
			return zero, domain.Invalid("exact unique condition source references")
		}
		seenRefs[source.Ref] = true
		for _, ref := range []*domain.GraphArtifactRef{source.GuidanceRef, source.AcceptanceRef} {
			if ref != nil && !validGraphArtifactReference(*ref) {
				return zero, domain.Invalid("complete condition originals")
			}
		}
		if source.AuthorityScope == "graph_shared" {
			shared++
			if source.NodeID != nil || source.TaskVersion != nil || source.GuidanceRef != nil || source.AcceptanceRef != nil {
				return zero, domain.Invalid("shared instructions without peer-node authority")
			}
		} else {
			if source.NodeID == nil || source.TaskVersion == nil || !domain.ValidID(*source.NodeID) || seenNodes[*source.NodeID] {
				return zero, domain.Invalid("attributed unique condition nodes")
			}
			if _, invalid := domain.ParseRevision(*source.TaskVersion); invalid != nil {
				return zero, invalid
			}
			seenNodes[*source.NodeID] = true
			switch source.AuthorityScope {
			case "receiver":
				receiver++
				if *source.TaskVersion != taskVersion {
					return zero, domain.Invalid("exact receiving condition version")
				}
			case "upstream_local":
				// Scope is explicit captured user provenance. Peer text cannot
				// turn its former obligations into permissions for the receiver.
			default:
				return zero, domain.Invalid("declared condition authority scope")
			}
		}
		sealed = append(sealed, cloneGraphConditionSource(source))
	}
	if shared != 1 || receiver != 1 {
		return zero, domain.Invalid("one shared and one receiving condition source")
	}
	sort.Slice(sealed, func(i, j int) bool { return sealed[i].Ref < sealed[j].Ref })
	binding := domain.GraphConditionsBinding{GraphID: graphID, GraphVersion: graphVersion, ReceiverTaskID: taskID, ReceiverTaskVersion: taskVersion, Sources: sealed}
	digest, err := domain.GraphConditionsDigest(binding)
	if err != nil {
		return zero, err
	}
	binding.ConditionsSHA256 = digest
	return binding, nil
}

func cloneGraphConditionSource(source domain.GraphConditionSource) domain.GraphConditionSource {
	if source.NodeID != nil {
		value := *source.NodeID
		source.NodeID = &value
	}
	if source.TaskVersion != nil {
		value := *source.TaskVersion
		source.TaskVersion = &value
	}
	if source.GuidanceRef != nil {
		value := *source.GuidanceRef
		source.GuidanceRef = &value
	}
	if source.AcceptanceRef != nil {
		value := *source.AcceptanceRef
		source.AcceptanceRef = &value
	}
	return source
}

// Preserve the ordinary node-condition digest separately. Every graph receiver
// uses this combined authority before workflow capture and direct-proof checks.
func graphEffectiveConditionsDigest(nodeDigest, graphDigest string) (string, error) {
	if !domain.ValidSHA256(nodeDigest) || !domain.ValidSHA256(graphDigest) {
		return "", domain.Invalid("complete node and scoped graph condition digests")
	}
	body, err := json.Marshal(struct {
		NodeConditionsSHA256  string `json:"nodeConditionsSha256"`
		GraphConditionsSHA256 string `json:"graphConditionsSha256"`
	}{nodeDigest, graphDigest})
	if err != nil {
		return "", err
	}
	return workflowHash(body), nil
}
