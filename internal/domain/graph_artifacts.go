package domain

// GraphArtifactRef names exact closed evidence without exposing its private path
// or physical identity. Physical closure is verified by the controller, not by
// possession of this serialized reference.
type GraphArtifactRef struct {
	ArtifactID        string `json:"artifactId"`
	ProducerAttemptID string `json:"producerAttemptId"`
	SHA256            string `json:"sha256"`
	ByteLength        string `json:"byteLength"`
}
