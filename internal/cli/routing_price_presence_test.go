package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRoutingCLIReferencePriceFileRejectsEmptyOptionalMembersBeforeMarshal(t *testing.T) {
	base := `"currency":"USD","sourceLabel":"User reference","observedAt":"2026-11-06T00:00:00Z","inputPerMillion":"1e999"`
	for _, field := range []string{"unit", "basis", "sourceUrl", "validUntil"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reference.json")
			raw := `{` + base + `,"` + field + `":""}`
			if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
				t.Fatal(e)
			}
			_, e := parse([]string{"routing", "profile", "set", "codex-luna-economy", "--expected-revision", "1", "--reference-price-file", path}, nil)
			if e == nil || e.Code != "invalid_request" {
				t.Fatal("an explicit empty file member disappeared before the HTTP request", field, e)
			}
		})
	}
}

func TestRoutingCLIReferencePriceFilePreservesOmissionAndExtremeLexeme(t *testing.T) {
	for _, fields := range []string{``, `,"unit":"per_million_tokens","basis":"api_list","sourceUrl":"https://example.invalid/reference","validUntil":"2099-01-01T00:00:00Z"`} {
		name := "omitted"
		if fields != "" {
			name = "explicit_nonempty"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reference.json")
			raw := `{"currency":"USD","sourceLabel":"User reference","observedAt":"2026-11-06T00:00:00Z","inputPerMillion":"1e999"` + fields + `}`
			if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
				t.Fatal(e)
			}
			c, e := parse([]string{"routing", "profile", "set", "codex-luna-economy", "--expected-revision", "1", "--reference-price-file", path}, nil)
			if e != nil {
				t.Fatal(e)
			}
			var in domain.ProfileUpdateInput
			if e := domain.StrictDecode(c.params.Body, &in); e != nil {
				t.Fatal(e)
			}
			if in.ReferencePrice == nil || in.ReferencePrice.InputPerMillion == nil || *in.ReferencePrice.InputPerMillion != "1e999" {
				t.Fatal("exact reference lexeme changed during CLI re-encoding", string(c.params.Body))
			}
			if fields == "" {
				for _, key := range []string{"unit", "basis", "sourceUrl", "validUntil"} {
					if strings.Contains(string(c.params.Body), `"`+key+`"`) {
						t.Fatal("omission was promoted into reference evidence", key, string(c.params.Body))
					}
				}
			}
			if fields != "" && (in.ReferencePrice.Unit != "per_million_tokens" || in.ReferencePrice.Basis != "api_list" || in.ReferencePrice.SourceURL != "https://example.invalid/reference" || in.ReferencePrice.ValidUntil != "2099-01-01T00:00:00Z") {
				t.Fatal("nonempty reference members changed", in.ReferencePrice)
			}
		})
	}
}
