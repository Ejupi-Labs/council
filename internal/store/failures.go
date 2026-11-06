package store

import (
	"encoding/json/jsontext"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func failureMetadata(f *domain.PrimaryFailure) jsontext.Value {
	if f == nil {
		f = &domain.PrimaryFailure{Code: "result_incomplete", Message: "A complete durable usable result with verified root exit and cleanup was not established.", ObservedAt: domain.Now()}
	}
	return jsontext.Value(encode(struct {
		Failure *domain.PrimaryFailure `json:"failure"`
	}{f}))
}
