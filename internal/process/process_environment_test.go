package process

import (
	"bytes"
	"context"
	"testing"
)

func TestOwnedEnvironmentPreservesInheritanceVersusExplicitEmpty(t *testing.T) {
	t.Setenv("COUNCIL_PROCESS_TEST_ENV_V1", "controlled-presence")
	for _, test := range []struct {
		name string
		env  []string
		want string
	}{
		{"inherited", nil, "present"},
		{"explicit_empty", []string{}, "absent"},
		{"explicit_selected", []string{"COUNCIL_PROCESS_TEST_ENV_V1=controlled-presence"}, "present"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			spec := selfSpec(t, "environment")
			spec.Env, spec.Stdout = test.env, &out
			owned, err := Prepare(context.Background(), spec)
			if err != nil {
				t.Fatal(err)
			}
			defer owned.Close()
			if err = owned.Start(); err != nil {
				t.Fatal(err)
			}
			root := waitRoot(t, owned)
			cleanupOwned(t, owned)
			if !root.Known || root.ExitCode != 0 || out.String() != test.want {
				t.Fatalf("owned environment contract: root=%+v sentinel=%q want=%q", root, out.String(), test.want)
			}
		})
	}
}
