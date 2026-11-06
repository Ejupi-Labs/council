package engine

import (
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

func persistenceFault(err error) bool {
	if hasErrorCode(err, "write_uncertain") {
		return true
	}
	return hasErrorCode(err, "storage_failed")
}

// Joined evidence errors may include more than one domain error. A commit with
// an uncertain acknowledgement always wins over an adjacent cancelled write.
func hasErrorCode(err error, code string) bool {
	if problem, ok := err.(*domain.Error); ok && problem != nil && problem.Code == code {
		return true
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			if hasErrorCode(child, code) {
				return true
			}
		}
	case interface{ Unwrap() error }:
		return hasErrorCode(wrapped.Unwrap(), code)
	}
	return false
}

func uncertainEvidence(err error) bool {
	if persistenceFault(err) || errors.Is(err, evidence.ErrCorrupt) || errors.Is(err, evidence.ErrDurability) {
		return true
	}
	return false
}
