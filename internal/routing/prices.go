// Package routing evaluates bounded declared policy without model, process, SQL or network work.
package routing

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// exactReferenceDecimal limits work before expansion. Native estimates and valid
// references outside this comparison domain remain exact opaque stored lexemes.
func exactReferenceDecimal(raw string) (*big.Rat, bool) {
	if len(raw) == 0 || len(raw) > 64 || raw[0] == '-' || strings.TrimSpace(raw) != raw {
		return nil, false
	}
	var number jsontext.Value
	if json.Unmarshal([]byte(raw), &number) != nil || number.Kind() != '0' {
		return nil, false
	}
	mantissa, exponent := raw, 0
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		written, err := strconv.ParseInt(raw[index+1:], 10, 32)
		if err != nil || written < -18 || written > 18 {
			return nil, false
		}
		exponent, mantissa = int(written), raw[:index]
	}
	fraction := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fraction = len(mantissa) - index - 1
		mantissa = mantissa[:index] + mantissa[index+1:]
	}
	coefficient := strings.TrimLeft(mantissa, "0")
	if coefficient == "" {
		return new(big.Rat), true
	}
	trailing := len(coefficient) - len(strings.TrimRight(coefficient, "0"))
	coefficient = coefficient[:len(coefficient)-trailing]
	scale := fraction - exponent - trailing
	if len(coefficient) > 32 || scale > 18 {
		return nil, false
	}
	integer, ok := new(big.Int).SetString(coefficient, 10)
	if !ok {
		return nil, false
	}
	if scale < 0 {
		integer.Mul(integer, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scale)), nil))
		return new(big.Rat).SetInt(integer), true
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	return new(big.Rat).SetFrac(integer, denominator), true
}

// ComparePrice returns -1 for a dominating lower declaration, 1 for a higher
// declaration, and 0 for equal rates. Crossed/missing evidence is incomparable.
func ComparePrice(a, b *domain.ReferencePrice, stamp string) (int, bool) {
	now, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || a == nil || b == nil || a.Currency != b.Currency || a.Unit == nil || b.Unit == nil || *a.Unit != "per_million_tokens" || *b.Unit != *a.Unit || a.Basis == nil || b.Basis == nil || (*a.Basis != "api_list" && *a.Basis != "declared_marginal") || *a.Basis != *b.Basis {
		return 0, false
	}
	rates := func(price *domain.ReferencePrice) (*big.Rat, *big.Rat, bool) {
		observed, err := time.Parse(time.RFC3339Nano, price.ObservedAt)
		if err != nil || observed.After(now) || price.SourceKind != "user_reference" || price.InputPerMillion == nil || price.OutputPerMillion == nil {
			return nil, nil, false
		}
		if price.ValidUntil != nil {
			expires, err := time.Parse(time.RFC3339Nano, *price.ValidUntil)
			if err != nil || !now.Before(expires) {
				return nil, nil, false
			}
		}
		input, inputOK := exactReferenceDecimal(*price.InputPerMillion)
		output, outputOK := exactReferenceDecimal(*price.OutputPerMillion)
		return input, output, inputOK && outputOK
	}
	ai, ao, aOK := rates(a)
	bi, bo, bOK := rates(b)
	if !aOK || !bOK {
		return 0, false
	}
	input, output := ai.Cmp(bi), ao.Cmp(bo)
	if input <= 0 && output <= 0 && (input < 0 || output < 0) {
		return -1, true
	}
	if input >= 0 && output >= 0 && (input > 0 || output > 0) {
		return 1, true
	}
	return 0, input == 0 && output == 0
}
