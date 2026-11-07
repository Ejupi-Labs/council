package workspace

import "github.com/Ejupi-Labs/council/v2/internal/domain"

// Supplemental sources are controller-selected closed evidence, not public
// instructions or additional permissions. Every source uses a private file copy.
type GraphContextSupplement struct {
	Ref         string                  `json:"ref"`
	ArtifactRef domain.GraphArtifactRef `json:"artifactRef"`
	Scope       string                  `json:"scope"`
	Paths       []string                `json:"paths"`
}

// This descriptor is a private file in the held per-attempt context directory.
// Its paths and bytes are never registered as a readable public artifact.
type GraphFinalContextDescriptor struct {
	Contract             string                   `json:"contract"`
	ReceivingAttemptID   string                   `json:"receivingAttemptId"`
	InputID              string                   `json:"inputId"`
	InputSHA256          string                   `json:"inputSha256"`
	PacketSHA256         string                   `json:"packetSha256"`
	FinalVerificationRef domain.GraphArtifactRef  `json:"finalVerificationRef"`
	Access               []GraphContextFileAccess `json:"access"`
}

type GraphContextFileAccess struct {
	Ref         string                  `json:"ref"`
	ArtifactRef domain.GraphArtifactRef `json:"artifactRef"`
	Scope       string                  `json:"scope"`
	Paths       []string                `json:"paths"`
	Path        string                  `json:"path"`
}
