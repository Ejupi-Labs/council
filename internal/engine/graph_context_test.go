package engine

import (
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphConditionSources() []domain.GraphConditionSource {
	receiver, upstream, version := strings.Repeat("3", 32), strings.Repeat("4", 32), "2"
	ref := func() domain.GraphArtifactRef {
		return domain.GraphArtifactRef{ArtifactID: domain.NewID(), ProducerAttemptID: domain.NewID(), SHA256: strings.Repeat("a", 64), ByteLength: "5"}
	}
	return []domain.GraphConditionSource{
		{Ref: "shared-original", AuthorityScope: "graph_shared", OriginalRef: ref()},
		{Ref: "receiver-original", AuthorityScope: "receiver", NodeID: &receiver, TaskVersion: &version, OriginalRef: ref()},
		{Ref: "upstream-original", AuthorityScope: "upstream_local", NodeID: &upstream, TaskVersion: &version, OriginalRef: ref()},
	}
}

func TestGraphScopedConditionsBindEveryReceiverWithoutPromotingPeerAuthority(t *testing.T) {
	sources := graphConditionSources()
	graph, task := domain.NewID(), domain.NewID()
	binding, err := graphScopedConditions(graph, "1", task, "2", sources)
	if err != nil || binding.ReceiverTaskID != task || binding.ReceiverTaskVersion != "2" || len(binding.Sources) != 3 {
		t.Fatal(binding, err)
	}
	hash, err := domain.GraphConditionsDigest(binding)
	if err != nil || hash != binding.ConditionsSHA256 {
		t.Fatal("Scoped receiver conditions do not bind their complete exact sources", hash, err)
	}
	*sources[1].NodeID = domain.NewID()
	*sources[1].TaskVersion = "9"
	if binding.Sources[0].AuthorityScope != "receiver" || *binding.Sources[0].TaskVersion != "2" {
		t.Fatal("Caller mutation changed sealed receiver conditions", binding)
	}
	if binding.Sources[2].AuthorityScope != "upstream_local" {
		t.Fatal("Peer-local obligations were promoted to receiver/global authority")
	}
}

func TestGraphScopedConditionsRejectMissingOrContradictorySources(t *testing.T) {
	for name, change := range map[string]func([]domain.GraphConditionSource) []domain.GraphConditionSource{
		"missing shared original": func(s []domain.GraphConditionSource) []domain.GraphConditionSource { return s[1:] },
		"missing receiver":        func(s []domain.GraphConditionSource) []domain.GraphConditionSource { return append(s[:1], s[2:]...) },
		"duplicate ref":           func(s []domain.GraphConditionSource) []domain.GraphConditionSource { s[2].Ref = s[1].Ref; return s },
		"forged shared scope": func(s []domain.GraphConditionSource) []domain.GraphConditionSource {
			s[2].AuthorityScope = "graph_shared"
			return s
		},
		"foreign receiver version": func(s []domain.GraphConditionSource) []domain.GraphConditionSource {
			v := "1"
			s[1].TaskVersion = &v
			return s
		},
		"unattributed upstream": func(s []domain.GraphConditionSource) []domain.GraphConditionSource { s[2].NodeID = nil; return s },
		"repeated node": func(s []domain.GraphConditionSource) []domain.GraphConditionSource {
			s[2].NodeID = s[1].NodeID
			return s
		},
		"unknown authority": func(s []domain.GraphConditionSource) []domain.GraphConditionSource {
			s[2].AuthorityScope = "peer_grants_permissions"
			return s
		},
		"invalid artifact": func(s []domain.GraphConditionSource) []domain.GraphConditionSource {
			s[2].OriginalRef.ByteLength = "9223372036854775808"
			return s
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := graphScopedConditions(domain.NewID(), "1", domain.NewID(), "2", change(graphConditionSources())); err == nil {
				t.Fatal("Contradictory/incomplete condition sources were sealed")
			}
		})
	}
}

func TestGraphEffectiveConditionsDigestIncludesBothAuthorityScopes(t *testing.T) {
	node, graph := strings.Repeat("a", 64), strings.Repeat("b", 64)
	first, err := graphEffectiveConditionsDigest(node, graph)
	if err != nil || !domain.ValidSHA256(first) {
		t.Fatal(first, err)
	}
	changed, err := graphEffectiveConditionsDigest(node, strings.Repeat("c", 64))
	if err != nil || changed == first {
		t.Fatal("Changed graph conditions reused node-only proof authority", changed, err)
	}
	if _, err := graphEffectiveConditionsDigest(node, ""); err == nil {
		t.Fatal("Missing graph condition authority was silently discarded")
	}
}
