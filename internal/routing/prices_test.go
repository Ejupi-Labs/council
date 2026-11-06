package routing

import "testing"

func TestBoundedExactReferenceDecimal(t *testing.T) {
	for _, tc := range []struct {
		raw, exact string
		ok         bool
	}{
		{"0", "0", true}, {"1.0000000000000000000000000000000000000000", "1", true},
		{"12345678901234567890123456789012", "12345678901234567890123456789012", true},
		{"123456789012345678901234567890123", "", false},
		{"1e18", "1000000000000000000", true}, {"1e-18", "1/1000000000000000000", true},
		{"1e19", "", false}, {"1e-19", "", false}, {"0.0000000000000000001", "", false},
		{"1e99999999999999999999999999999999999999999999999999999", "", false},
		{"0.0100e+2", "1", true}, {"1e00018", "1000000000000000000", true},
		{"-0", "", false}, {"01", "", false}, {"NaN", "", false}, {" 1", "", false}, {"1 ", "", false},
		{".1", "", false}, {"1.", "", false}, {"1e", "", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			value, ok := exactReferenceDecimal(tc.raw)
			if ok != tc.ok || ok && value.RatString() != tc.exact {
				t.Fatalf("comparable domain changed for %q: %v %v", tc.raw, value, ok)
			}
		})
	}
}
