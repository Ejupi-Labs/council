package schemas

import "testing"

func TestApprovalRequiresExactReviewedDigest(t *testing.T) {
	const name = "council_owner_meta"
	expected, ok := approved[name]
	if !ok {
		t.Fatal("reviewed schema missing")
	}
	if _, ok := Lookup(name); !ok {
		t.Fatal("reviewed digest rejected")
	}
	approved[name] = "not-the-reviewed-digest"
	t.Cleanup(func() { approved[name] = expected })
	if value, ok := Lookup(name); ok || value != nil {
		t.Fatal("changed or unreviewed schema was exposed")
	}
}
