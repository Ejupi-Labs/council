//go:build windows

package localpath

import "testing"

func TestPhysicalPathUTF16IsLossless(t *testing.T) {
	for _, tc := range []struct {
		name  string
		units []uint16
		want  string
		valid bool
	}{
		{"ASCII", []uint16{'C', ':', '\\', 'x'}, `C:\x`, true},
		{"supplementary scalar", []uint16{0xd83d, 0xde00}, "😀", true},
		{"literal replacement character", []uint16{0xfffd}, "�", true},
		{"unpaired high surrogate", []uint16{0xd800}, "", false},
		{"unpaired low surrogate", []uint16{0xdc00}, "", false},
		{"interior NUL", []uint16{'a', 0, 'b'}, "", false},
		{"trailing NUL", []uint16{'a', 0}, "", false},
		{"empty path", nil, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodePath(tc.units)
			if tc.valid {
				if err != nil || got != tc.want {
					t.Fatalf("valid path changed: got=%q err=%v", got, err)
				}
			} else if err == nil {
				t.Fatalf("lossy path accepted: %q", got)
			}
		})
	}
}
