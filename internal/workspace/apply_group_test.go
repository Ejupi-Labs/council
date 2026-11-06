package workspace

import (
	"errors"
	"testing"
)

func TestApplyLinuxNewFileGroupRejectsAmbiguousInheritance(t *testing.T) {
	// Pure typed native facts only: no mount, credential query or privileged
	// fixture. The ordinary parent lacks setgid, so a differing parent/effective
	// group cannot distinguish Linux sysvgroups from grpid/bsdgroups semantics.
	group, err := applyLinuxNewFileGroup(2401, 1907, false)
	if !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("unestablished mount-specific group rule was accepted: group=%d error=%v", group, err)
	}
}

func TestApplyLinuxNewFileGroupPreservesUnambiguousRules(t *testing.T) {
	for _, test := range []struct {
		name              string
		parent, effective uint32
		setgid            bool
		want              uint32
	}{
		{name: "equal parent and effective group", parent: 1907, effective: 1907, want: 1907},
		{name: "setgid supplies distinct parent group", parent: 2401, effective: 1907, setgid: true, want: 2401},
		{name: "setgid with matching group", parent: 2401, effective: 2401, setgid: true, want: 2401},
	} {
		t.Run(test.name, func(t *testing.T) {
			group, err := applyLinuxNewFileGroup(test.parent, test.effective, test.setgid)
			if err != nil || group != test.want {
				t.Fatalf("supported explicit group rule changed: got=%d want=%d error=%v", group, test.want, err)
			}
		})
	}
}
