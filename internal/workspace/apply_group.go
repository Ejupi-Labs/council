package workspace

import "fmt"

// applyLinuxNewFileGroup is a pure decision over observed native group facts.
// Keeping it portable permits regression of the admission rule on every test
// host; these typed-fact cases do not qualify an actual mounted filesystem.
func applyLinuxNewFileGroup(parentGID, effectiveGID uint32, parentSetgid bool) (uint32, error) {
	if parentSetgid {
		return parentGID, nil
	}
	// Linux also permits grpid/bsdgroups mounts, which inherit the parent
	// group without setgid. A mount identity alone does not establish that
	// creation rule. Both rules agree only when these group values agree.
	// https://docs.kernel.org/admin-guide/ext4.html#options
	if parentGID != effectiveGID {
		return 0, fmt.Errorf("%w A differing destination group needs an established native creation rule.", ErrApplyUnsupported)
	}
	return effectiveGID, nil
}
