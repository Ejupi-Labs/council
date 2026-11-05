//go:build darwin || linux

package discovery

// Unix CLI installations are exposed through PATH; no vendor home/config trees are inspected.
func nativeCandidates(string) ([]string, error) { return nil, nil }
